package excel_parser_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/init-pkg/nova-template/domain/app"
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

type ExcelParserService struct {
	openaiClient *openai.Client
	log          *slog.Logger
	cache        HeaderCache
}

var _ app.ExcelParserService = &ExcelParserService{}

func New(openaiClient *openai.Client, log *slog.Logger) *ExcelParserService {
	return &ExcelParserService{openaiClient, log, &memHeaderCache{}}
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
		prompt := "Analyze this Excel data and identify which row contains the actual table column headers and where the data starts. " +
			"Look for rows with column names like 'Бренд', 'Артикул', 'Наименование', 'Цена', 'Остаток', etc. " +
			"Ignore contact info, metadata, and empty rows.\n\nRows:\n"

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

		headerRows := 1 // Default fallback
		actualHeaderRowIndex := -1
		actualDataStartIndex := -1

		if len(resp.Choices) > 0 {
			gptResponse := strings.TrimSpace(resp.Choices[0].Message.Content)

			// Parse JSON response directly (no markdown blocks with structured outputs)
			var analysis GPTAnalysisResponse
			if parseErr := json.Unmarshal([]byte(gptResponse), &analysis); parseErr == nil {
				if analysis.HeaderRowIndex >= 0 && analysis.HeaderRowIndex < len(rowTexts) &&
					analysis.DataStartIndex >= 0 && analysis.DataStartIndex < len(rowTexts) &&
					analysis.DataStartIndex > analysis.HeaderRowIndex {
					// Convert rowTexts indices to actual grid indices
					actualHeaderRowIndex = rowIndices[analysis.HeaderRowIndex]
					actualDataStartIndex = rowIndices[analysis.DataStartIndex]
				}
			} else {
				// Fallback: try to parse as plain number
				if parsed, parseErr := strconv.Atoi(strings.TrimSpace(gptResponse)); parseErr == nil && parsed > 0 && parsed <= len(rowTexts) {
					headerRows = parsed
				}
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
							// Check if this looks like a header (short, likely column name) vs data/category
							// Skip very long values that are likely category names or data
							if len(val) <= 50 && !strings.Contains(val, " ") {
								headerValue = val
								break
							}
						}
					}
				}
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
