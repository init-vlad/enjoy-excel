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
			results = append(results, result)
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
			results = append(results, result)
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

		results = append(results, result)
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
			copy(grid[i], rows[i])
		}
	}

	merges, err := f.GetMergeCells(sheet)
	if err != nil {
		return nil, 0, err
	}
	for _, merge := range merges {
		val := merge[0] // Value is first element
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
		// Use multiple rows from header to data start for complex headers
		for c := 0; c < maxCol; c++ {
			var headerParts []string
			for r := actualHeaderRowIndex; r < actualDataStartIndex && r < len(grid); r++ {
				if c < len(grid[r]) {
					val := strings.TrimSpace(grid[r][c])
					if val != "" {
						headerParts = append(headerParts, val)
					}
				}
			}
			if len(headerParts) > 0 {
				header[c] = strings.Join(headerParts, " ")
			}
		}
	} else {
		// Fallback to original logic if no specific header row identified
		for c := 0; c < maxCol; c++ {
			for r := 0; r < headerRows; r++ {
				val := strings.TrimSpace(grid[startRow+r][c])
				if val != "" {
					header[c] = val
				}
			}
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
			row[j] = grid[i][j]
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
