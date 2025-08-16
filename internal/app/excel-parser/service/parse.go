package excel_parser_service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/init-pkg/nova-template/domain/app"
	app_redis "github.com/init-pkg/nova-template/internal/infra/redis/client"
	"github.com/init-pkg/nova/errs"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"

	"github.com/invopop/jsonschema"
	openai "github.com/openai/openai-go/v2"
	"github.com/xuri/excelize/v2"
)

type GPTAnalysisResponse struct {
	HeaderRowIndex int    `json:"header_row_index" jsonschema_description:"Index of the row in the provided list that contains the actual table column headers (like Бренд, Артикул, etc.). This is relative to the rows provided in the prompt, starting from 0."`
	DataStartIndex int    `json:"data_start_index" jsonschema_description:"Index of the first row in the provided list that contains actual table data. Must be greater than header_row_index."`
	Reasoning      string `json:"reasoning" jsonschema_description:"Brief explanation of the analysis and why these row indices were chosen"`
}

type GPTTableValidationResponse struct {
	IsProductTable bool   `json:"is_product_table" jsonschema_description:"Whether this appears to be a product/goods table (has columns like артикул, цена, остаток, etc.)"`
	Confidence     int    `json:"confidence" jsonschema_description:"Confidence level from 1-10, where 10 means definitely a product table"`
	Reasoning      string `json:"reasoning" jsonschema_description:"Brief explanation of why this is or isn't a product table"`
}

func GenerateSchema[T any]() interface{} {
	// Structured Outputs uses a subset of JSON schema
	// These flags are necessary to comply with the subset
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return schema
}

// Generate the JSON schema at initialization time
var GPTAnalysisResponseSchema = GenerateSchema[GPTAnalysisResponse]()
var GPTTableValidationResponseSchema = GenerateSchema[GPTTableValidationResponse]()

// Cache constants
const (
	cacheTTL                = 30 * 24 * time.Hour // 30 days
	tableValidationCacheKey = "excel_parser:table_validation:"
	headerAnalysisCacheKey  = "excel_parser:header_analysis:"
)

type ExcelParserService struct {
	openaiClient *openai.Client
	log          *slog.Logger
	cache        HeaderCache
	redisClient  *app_redis.Client
}

var _ app.ExcelParserService = &ExcelParserService{}

func New(openaiClient *openai.Client, log *slog.Logger, redisClient *app_redis.Client) *ExcelParserService {
	return &ExcelParserService{openaiClient, log, &memHeaderCache{}, redisClient}
}

// generateCacheKey creates a cache key based on header data
func (this *ExcelParserService) generateCacheKey(prefix string, headers []string) string {
	// Create a hash from headers to ensure consistent cache keys
	headerText := strings.Join(headers, "|")
	hash := md5.Sum([]byte(headerText))
	return prefix + hex.EncodeToString(hash[:])
}

// generateTableValidationCacheKey creates a cache key for table validation including limited sample data context
func (this *ExcelParserService) generateTableValidationCacheKey(headers []string, sampleRows [][]string) string {
	// Include headers in the key
	var keyParts []string
	keyParts = append(keyParts, strings.Join(headers, "|"))

	// Add limited sample data context (only first few cells of first few rows)
	// This captures the "flavor" of data without making cache too specific
	for i, row := range sampleRows {
		if i >= 2 { // Limit to first 2 sample rows
			break
		}
		// Only include first 3 cells to get data "type pattern"
		limitedRow := make([]string, 0, 3)
		for j, cell := range row {
			if j >= 3 { // Only first 3 cells
				break
			}
			// Normalize data to detect patterns rather than specific values
			normalized := this.normalizeDataForCaching(cell)
			limitedRow = append(limitedRow, normalized)
		}
		keyParts = append(keyParts, strings.Join(limitedRow, "|"))
	}

	keyText := strings.Join(keyParts, "||")
	hash := md5.Sum([]byte(keyText))
	return tableValidationCacheKey + hex.EncodeToString(hash[:])
}

// normalizeDataForCaching normalizes data values to detect patterns rather than cache specific values
func (this *ExcelParserService) normalizeDataForCaching(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "EMPTY"
	}

	// Check if it's a number
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return "NUMBER"
	}

	// Check if it looks like a product code/SKU (alphanumeric)
	if this.isAlphanumericCode(value) {
		return "CODE"
	}

	// Check length categories for text
	if len(value) <= 10 {
		return "SHORT_TEXT"
	} else if len(value) <= 50 {
		return "MEDIUM_TEXT"
	} else {
		return "LONG_TEXT"
	}
}

// isAlphanumericCode checks if string looks like a product code
func (this *ExcelParserService) isAlphanumericCode(s string) bool {
	if len(s) < 2 || len(s) > 20 {
		return false
	}

	hasLetter := false
	hasDigit := false

	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			hasLetter = true
		} else if r >= '0' && r <= '9' {
			hasDigit = true
		} else if r != '-' && r != '_' && r != '.' {
			return false // Invalid character for product code
		}
	}

	return hasLetter || hasDigit // At least one letter or digit
}

// getHeuristicHeaders attempts to determine likely headers without GPT analysis
func (this *ExcelParserService) getHeuristicHeaders(grid [][]string, startRow, maxCol int) []string {
	// Use ONLY the first meaningful row as headers for caching consistency
	headers := make([]string, maxCol)

	if startRow < len(grid) {
		for c := 0; c < maxCol && c < len(grid[startRow]); c++ {
			headers[c] = strings.TrimSpace(grid[startRow][c])
		}
	}

	return headers
}

// getHeuristicSampleRows gets sample data rows (excluding the header row)
func (this *ExcelParserService) getHeuristicSampleRows(grid [][]string, startRow, maxCol int) [][]string {
	sampleRows := make([][]string, 0, 3)

	// Start from startRow+1 to skip the header row, take up to 3 data rows
	for i := startRow + 1; i < min(startRow+4, len(grid)); i++ {
		if i < len(grid) {
			row := make([]string, maxCol)
			hasData := false
			for j := 0; j < maxCol && j < len(grid[i]); j++ {
				if j < len(grid[i]) {
					row[j] = strings.TrimSpace(grid[i][j])
					if row[j] != "" {
						hasData = true
					}
				}
			}
			if hasData {
				sampleRows = append(sampleRows, row)
			}
		}
	}

	return sampleRows
}

// getCachedTableValidation retrieves cached table validation result
func (this *ExcelParserService) getCachedTableValidation(headers []string, sampleRows [][]string) (*GPTTableValidationResponse, bool) {
	cacheKey := this.generateTableValidationCacheKey(headers, sampleRows)

	result := this.redisClient.Get(context.Background(), cacheKey)
	if result.Err() != nil {
		return nil, false
	}

	data, err := result.Result()
	if err != nil {
		return nil, false
	}

	var validation GPTTableValidationResponse
	if err := json.Unmarshal([]byte(data), &validation); err != nil {
		return nil, false
	}

	return &validation, true
}

// setCachedTableValidation stores table validation result in cache
func (this *ExcelParserService) setCachedTableValidation(headers []string, sampleRows [][]string, validation *GPTTableValidationResponse) {
	cacheKey := this.generateTableValidationCacheKey(headers, sampleRows)

	data, err := json.Marshal(validation)
	if err != nil {
		this.log.Error("failed to marshal table validation for cache", "error", err)
		return
	}

	if err := this.redisClient.Set(context.Background(), cacheKey, string(data), cacheTTL).Err(); err != nil {
		this.log.Error("failed to cache table validation", "error", err)
	} else {
		this.log.Debug("Cached table validation result", "cacheKey", cacheKey[:20]+"...")
	}
}

// getCachedHeaderAnalysis retrieves cached header analysis result
func (this *ExcelParserService) getCachedHeaderAnalysis(rowTexts []string) (*GPTAnalysisResponse, bool) {
	cacheKey := this.generateCacheKey(headerAnalysisCacheKey, rowTexts)

	result := this.redisClient.Get(context.Background(), cacheKey)
	if result.Err() != nil {
		return nil, false
	}

	data, err := result.Result()
	if err != nil {
		return nil, false
	}

	var analysis GPTAnalysisResponse
	if err := json.Unmarshal([]byte(data), &analysis); err != nil {
		return nil, false
	}

	return &analysis, true
}

// setCachedHeaderAnalysis stores header analysis result in cache
func (this *ExcelParserService) setCachedHeaderAnalysis(rowTexts []string, analysis *GPTAnalysisResponse) {
	cacheKey := this.generateCacheKey(headerAnalysisCacheKey, rowTexts)

	data, err := json.Marshal(analysis)
	if err != nil {
		this.log.Error("failed to marshal header analysis for cache", "error", err)
		return
	}

	if err := this.redisClient.Set(context.Background(), cacheKey, string(data), cacheTTL).Err(); err != nil {
		this.log.Error("failed to cache header analysis", "error", err)
	} else {
		this.log.Debug("Cached header analysis result", "cacheKey", cacheKey[:20]+"...")
	}
}

func (this *ExcelParserService) Parse(ctx nova_ctx.Ctx, file []byte) ([]*app.ParseExcelResult, errs.Error) {
	res, err := this.parse(file)
	if err != nil {
		return nil, errs.WrapAppError(err, &errs.ErrorOpts{})
	}

	return res, nil
}

// isProductTable checks if the given table structure represents a product/goods table
func (this *ExcelParserService) isProductTable(header []string, sampleRows [][]string) bool {
	// Check cache first
	if cached, found := this.getCachedTableValidation(header, sampleRows); found {
		this.log.Info("Using cached table validation result",
			"isProductTable", cached.IsProductTable,
			"confidence", cached.Confidence,
			"reasoning", cached.Reasoning)
		return cached.IsProductTable && cached.Confidence >= 7
	}

	// Create a prompt with header and sample data
	headerText := strings.Join(header, " | ")

	prompt := "Analyze this table structure and determine if it represents a product/goods catalog table. " +
		"Look for typical e-commerce/catalog columns like артикул, код, цена, остаток, наименование, описание, бренд, etc. " +
		"Ignore contact information, navigation menus, or administrative tables.\n\n" +
		"Header: " + headerText + "\n\n"

	if len(sampleRows) > 0 {
		prompt += "Sample data rows:\n"
		for i, row := range sampleRows {
			if i >= 3 { // Limit to first 3 rows
				break
			}
			rowText := strings.Join(row, " | ")
			prompt += fmt.Sprintf("Row %d: %s\n", i+1, rowText)
		}
	}

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "table_validation_response",
		Description: openai.String("Analysis of whether table represents product catalog"),
		Schema:      GPTTableValidationResponseSchema,
		Strict:      openai.Bool(true),
	}

	req := openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT5Nano,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	}

	resp, err := this.openaiClient.Chat.Completions.New(context.Background(), req)
	if err != nil {
		this.log.Error("failed to validate table with GPT", "error", err)
		return true // Default to true if validation fails
	}

	if len(resp.Choices) > 0 {
		gptResponse := strings.TrimSpace(resp.Choices[0].Message.Content)
		var validation GPTTableValidationResponse
		if parseErr := json.Unmarshal([]byte(gptResponse), &validation); parseErr == nil {
			// Cache the result
			this.setCachedTableValidation(header, sampleRows, &validation)

			this.log.Info("GPT table validation result",
				"isProductTable", validation.IsProductTable,
				"confidence", validation.Confidence,
				"reasoning", validation.Reasoning)
			return validation.IsProductTable && validation.Confidence >= 7
		}
	}

	return true // Default to true if parsing fails
}

type HeaderCache interface {
	// Assuming some interface, but not used in this implementation
}

type memHeaderCache struct {
	// Implementation if needed
}

func (this *ExcelParserService) parse(file []byte) ([]*app.ParseExcelResult, error) {
	this.log.Info("Excel parsing started")

	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}

	var results []*app.ParseExcelResult
	for _, sheet := range f.GetSheetList() {
		grid, maxCol, err := getFilledGrid(f, sheet)
		if err != nil {
			this.log.Error("failed to get filled grid", "error", err)
			continue
		}
		if len(grid) == 0 {
			continue
		}

		// Find start row: first row with at least 3 non-empty cells
		startRow := -1
		for i := 0; i < len(grid); i++ {
			nonEmptyCount := 0
			for _, cell := range grid[i] {
				if cell != "" {
					nonEmptyCount++
				}
			}
			if nonEmptyCount >= 3 {
				startRow = i
				break
			}
		}
		if startRow == -1 {
			continue
		}

		// Early cache check: try to determine if this is a product table using heuristic headers
		// This avoids GPT calls for structure analysis when we already know the answer
		heuristicHeaders := this.getHeuristicHeaders(grid, startRow, maxCol)
		heuristicSampleRows := this.getHeuristicSampleRows(grid, startRow, maxCol)

		// Check cache for table validation using heuristic headers
		if cached, found := this.getCachedTableValidation(heuristicHeaders, heuristicSampleRows); found {
			this.log.Info("Using cached table validation for early decision",
				"sheet", sheet,
				"isProductTable", cached.IsProductTable,
				"confidence", cached.Confidence)

			// If cache says it's not a product table with high confidence, skip expensive processing
			if !cached.IsProductTable && cached.Confidence >= 7 {
				this.log.Info("Skipping sheet based on cached validation - not a product table", "sheet", sheet)
				continue
			}
		}

		// Determine header rows using GPT
		potentialHeaderRows := min(10, len(grid)-startRow)
		rowTexts := make([]string, 0, potentialHeaderRows)
		rowIndices := make([]int, 0, potentialHeaderRows) // Track original indices
		for j := 0; j < potentialHeaderRows; j++ {
			text := strings.Join(grid[startRow+j][:maxCol], " ")
			if strings.TrimSpace(text) == "" {
				continue
			}
			rowTexts = append(rowTexts, text)
			rowIndices = append(rowIndices, startRow+j) // Store original row index
		}

		if len(rowTexts) < 2 {
			// Too few rows, assume no header or minimal
			headerRows := 0
			if len(rowTexts) == 1 {
				headerRows = 1
			}
			result := buildResultWithIndices(grid, startRow, headerRows, -1, -1, maxCol)

			// Still validate even minimal tables
			sampleRows := result.Rows
			if len(sampleRows) > 3 {
				sampleRows = sampleRows[:3]
			}

			if this.isProductTable(result.Header, sampleRows) {
				this.log.Info("Minimal table validated as product table")
				results = append(results, result)
			} else {
				this.log.Info("Minimal table rejected - not a product table", "header", result.Header)
			}
			continue
		}

		// Use GPT to analyze header structure
		// Check cache first
		var analysis GPTAnalysisResponse
		var useGPTResult bool

		if cached, found := this.getCachedHeaderAnalysis(rowTexts); found {
			this.log.Info("Using cached header analysis result", "headerRowIndex", cached.HeaderRowIndex, "dataStartIndex", cached.DataStartIndex)
			analysis = *cached
			useGPTResult = true
		} else {
			prompt := "Analyze this Excel data and identify which row contains the actual table column headers and where the data starts.\n\n" +
				"IMPORTANT RULES:\n" +
				"1. Column headers are typically short, descriptive field names (like product codes, names, prices, quantities)\n" +
				"2. Avoid rows that contain category names, section titles, or descriptive text that spans multiple cells\n" +
				"3. If you see a row with long descriptive text followed by a row with short field-like names, choose the row with short field names\n" +
				"4. Data rows contain actual values, not field names\n" +
				"5. Skip any promotional text, contact information, or metadata\n\n" +
				"Example patterns:\n" +
				"- GOOD header row: 'Code | Name | Price | Stock'\n" +
				"- BAD header row: 'Electronics and Computer Accessories for Modern Office'\n" +
				"- GOOD data row: 'A123 | Laptop Dell | 1500.00 | 5'\n\n" +
				"Rows:\n"

			for i, text := range rowTexts {
				prompt += fmt.Sprintf("Row %d: %s\n", i, text)
			}

			schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "gpt_analysis_response",
				Description: openai.String("Analysis of Excel data to determine header rows"),
				Schema:      GPTAnalysisResponseSchema,
				Strict:      openai.Bool(true),
			}

			req := openai.ChatCompletionNewParams{
				Model: openai.ChatModelGPT5Nano,
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage(prompt),
				},
				ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
					OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
						JSONSchema: schemaParam,
					},
				},
			}

			resp, err := this.openaiClient.Chat.Completions.New(context.Background(), req)
			if err != nil {
				this.log.Error("failed to get GPT response", "error", err)
				// Fallback to heuristic: assume 1-2 header rows
				result := buildResultWithIndices(grid, startRow, 1, -1, -1, maxCol)

				// Still validate fallback tables
				sampleRows := result.Rows
				if len(sampleRows) > 3 {
					sampleRows = sampleRows[:3]
				}

				if this.isProductTable(result.Header, sampleRows) {
					this.log.Info("Fallback table validated as product table")
					results = append(results, result)
				} else {
					this.log.Info("Fallback table rejected - not a product table", "header", result.Header)
				}
				continue
			}

			if len(resp.Choices) > 0 {
				gptResponse := strings.TrimSpace(resp.Choices[0].Message.Content)

				// Parse JSON response directly (no markdown blocks with structured outputs)
				if parseErr := json.Unmarshal([]byte(gptResponse), &analysis); parseErr == nil {
					// Cache the successful result
					this.setCachedHeaderAnalysis(rowTexts, &analysis)
					useGPTResult = true
				}
			}
		}

		headerRows := 1 // Default fallback
		actualHeaderRowIndex := -1
		actualDataStartIndex := -1

		if useGPTResult {
			if analysis.HeaderRowIndex >= 0 && analysis.HeaderRowIndex < len(rowTexts) &&
				analysis.DataStartIndex >= 0 && analysis.DataStartIndex < len(rowTexts) &&
				analysis.DataStartIndex > analysis.HeaderRowIndex {
				// Convert rowTexts indices to actual grid indices
				actualHeaderRowIndex = rowIndices[analysis.HeaderRowIndex]
				actualDataStartIndex = rowIndices[analysis.DataStartIndex]
			}
		}

		// Cap at reasonable number
		if headerRows > 5 {
			headerRows = 3 // Reasonable fallback for complex headers
		}

		result := buildResultWithIndices(grid, startRow, headerRows, actualHeaderRowIndex, actualDataStartIndex, maxCol)

		// Log detailed information about the parsed table structure
		this.log.Info("Parsed table structure",
			"sheet", sheet,
			"startRow", startRow,
			"headerRows", headerRows,
			"actualHeaderRowIndex", actualHeaderRowIndex,
			"actualDataStartIndex", actualDataStartIndex,
			"maxCol", maxCol,
			"header", result.Header,
			"rowCount", len(result.Rows))

		// Check if this is a product table using GPT
		sampleRows := result.Rows
		if len(sampleRows) > 3 {
			sampleRows = sampleRows[:3] // Limit to first 3 rows for analysis
		}

		if this.isProductTable(result.Header, sampleRows) {
			this.log.Info("Table validated as product table", "sheet", sheet)
			results = append(results, result)
		} else {
			this.log.Info("Table rejected - not a product table", "sheet", sheet, "header", result.Header)
		}
	}

	this.log.Info("Excel parsing completed successfully", "sheetsProcessed", len(results))
	return results, nil
}

func getFilledGrid(f *excelize.File, sheet string) ([][]string, int, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	maxCol := 0
	for _, row := range rows {
		if len(row) > maxCol {
			maxCol = len(row)
		}
	}

	grid := make([][]string, len(rows))
	for i := range grid {
		grid[i] = make([]string, maxCol)
		if len(rows[i]) > 0 {
			// Apply trim to all cell values
			for j, cell := range rows[i] {
				if j < maxCol {
					grid[i][j] = strings.TrimSpace(cell)
				}
			}
		}
	}

	merges, err := f.GetMergeCells(sheet)
	if err != nil {
		return nil, 0, err
	}
	for _, merge := range merges {
		val := strings.TrimSpace(merge[0]) // Apply trim to merged cell values too
		// Parse range
		parts := strings.Split(merge[1], ":")
		if len(parts) != 2 {
			continue
		}
		startCol, startRowIdx, err := excelize.CellNameToCoordinates(parts[0])
		if err != nil {
			continue
		}
		endCol, endRowIdx, err := excelize.CellNameToCoordinates(parts[1])
		if err != nil {
			continue
		}
		startCol--
		startRowIdx--
		endCol--
		endRowIdx--
		for r := startRowIdx; r <= endRowIdx; r++ {
			for c := startCol; c <= endCol; c++ {
				if r < len(grid) && c < len(grid[r]) {
					grid[r][c] = val
				}
			}
		}
	}

	return grid, maxCol, nil
}

func buildResultWithIndices(grid [][]string, startRow, headerRows, actualHeaderRowIndex, actualDataStartIndex, maxCol int) *app.ParseExcelResult {
	// Build header using specific header row index if provided
	header := make([]string, maxCol)

	if actualHeaderRowIndex >= 0 && actualDataStartIndex >= 0 {
		// When we have a specific header row, use it directly first, then fill gaps with subsequent rows
		for c := 0; c < maxCol; c++ {
			var headerValue string

			// First, try to get the value from the specific header row
			if actualHeaderRowIndex < len(grid) && c < len(grid[actualHeaderRowIndex]) {
				headerValue = strings.TrimSpace(grid[actualHeaderRowIndex][c])
			}

			// If the header row is empty for this column, look in subsequent rows until data starts
			// but stop if we find data-like content (like category names)
			if headerValue == "" {
				for r := actualHeaderRowIndex + 1; r < actualDataStartIndex && r < len(grid); r++ {
					if c < len(grid[r]) {
						val := strings.TrimSpace(grid[r][c])
						if val != "" {
							// Check if this looks like a header vs category/data
							// Skip very long values (likely category names) but allow normal headers with spaces
							if len(val) <= 30 {
								headerValue = val
								break
							}
						}
					}
				}
			}

			// Dynamic header quality improvement: if we have a short/ambiguous header,
			// look for better alternatives in the same column
			if headerValue != "" && (len(headerValue) <= 4 || isNumericOrCurrency(headerValue)) {
				bestAlternative := headerValue
				bestScore := scoreHeaderQuality(headerValue)

				// Search in all rows between header and data start for better alternatives
				for r := actualHeaderRowIndex; r < actualDataStartIndex && r < len(grid); r++ {
					if c < len(grid[r]) {
						candidate := strings.TrimSpace(grid[r][c])
						if candidate != "" && candidate != headerValue {
							candidateScore := scoreHeaderQuality(candidate)
							if candidateScore > bestScore {
								bestAlternative = candidate
								bestScore = candidateScore
							}
						}
					}
				}

				headerValue = bestAlternative
			}

			header[c] = headerValue
		}
	} else {
		// Fallback to original logic if no specific header row identified
		// Also use "last non-empty" logic here for consistency
		for c := 0; c < maxCol; c++ {
			var lastNonEmptyValue string
			for r := 0; r < headerRows; r++ {
				if startRow+r < len(grid) && c < len(grid[startRow+r]) {
					val := strings.TrimSpace(grid[startRow+r][c])
					if val != "" {
						lastNonEmptyValue = val
					}
				}
			}
			header[c] = lastNonEmptyValue
		}
	}

	// Build rows starting from data start index if provided, otherwise use headerRows
	var rows [][]string
	dataStart := startRow + headerRows
	if actualDataStartIndex >= 0 {
		dataStart = actualDataStartIndex
	}

	for i := dataStart; i < len(grid); i++ {
		row := make([]string, maxCol)
		empty := true
		for j := 0; j < maxCol && j < len(grid[i]); j++ {
			row[j] = strings.TrimSpace(grid[i][j]) // Apply trim here too
			if row[j] != "" {
				empty = false
			}
		}
		if !empty {
			rows = append(rows, row)
		}
	}

	return &app.ParseExcelResult{
		Header: header,
		Rows:   rows,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// scoreHeaderQuality gives a quality score to a potential header string
// Higher score means better header quality
func scoreHeaderQuality(header string) int {
	if header == "" {
		return 0
	}

	score := 0

	// Length scoring: prefer meaningful length headers
	length := len(header)
	if length >= 3 && length <= 15 {
		score += 3 // Good length range for column names
	} else if length > 15 && length <= 30 {
		score += 1 // Acceptable but getting long
	} else if length < 3 {
		score -= 2 // Too short, likely abbreviation
	} else {
		score -= 4 // Too long, likely category/section name
	}

	// Content analysis
	wordCount := len(strings.Fields(header))
	if wordCount == 1 {
		score += 2 // Single words are often good column names
	} else if wordCount == 2 {
		score += 1 // Two words can be good (like "Full Name")
	} else if wordCount > 3 {
		score -= 3 // Too many words, likely descriptive text
	}

	// Penalize pure numbers or currency symbols
	if isNumericOrCurrency(header) {
		score -= 5
	}

	// Penalize strings that look like sentences or descriptions
	if strings.Contains(header, ".") || strings.Contains(header, ",") || strings.Contains(header, ":") {
		score -= 3
	}

	// Bonus for typical column name patterns (not specific words, but patterns)
	if looksLikeColumnName(header) {
		score += 3
	}

	return score
}

// isNumericOrCurrency checks if string is purely numeric or currency-like
func isNumericOrCurrency(s string) bool {
	if s == "" {
		return false
	}

	// Check for pure numbers
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}

	// Check for common currency patterns (3-4 letter codes)
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 3 || len(s) == 4 {
		// Check if it's all letters (likely currency code)
		allLetters := true
		for _, r := range s {
			if r < 'A' || r > 'Z' {
				allLetters = false
				break
			}
		}
		if allLetters {
			return true
		}
	}

	// Check for currency symbols
	currencySymbols := []string{"$", "€", "₽", "¥", "£"}
	for _, symbol := range currencySymbols {
		if strings.Contains(s, symbol) {
			return true
		}
	}

	return false
}

// looksLikeColumnName uses pattern analysis to detect column-like names
func looksLikeColumnName(s string) bool {
	if s == "" {
		return false
	}

	// Pattern 1: Single word that's not too long
	words := strings.Fields(s)
	if len(words) == 1 && len(s) >= 2 && len(s) <= 12 {
		return true
	}

	// Pattern 2: Two short words (like "First Name", "Item Code")
	if len(words) == 2 {
		word1, word2 := words[0], words[1]
		if len(word1) <= 8 && len(word2) <= 8 {
			return true
		}
	}

	// Pattern 3: Contains common column suffixes/prefixes
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "id") || strings.HasSuffix(lower, "code") ||
		strings.HasSuffix(lower, "name") || strings.HasSuffix(lower, "number") ||
		strings.HasPrefix(lower, "num") || strings.HasPrefix(lower, "qty") {
		return true
	}

	return false
}
