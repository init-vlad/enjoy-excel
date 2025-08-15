package excel_parser_service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"strings"

	"github.com/init-pkg/nova-template/domain/app"
	"github.com/init-pkg/nova/errs"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"

	openai "github.com/openai/openai-go/v2"
	"github.com/xuri/excelize/v2"
)

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

	js, err := json.Marshal(res)
	if err != nil {
		return nil, errs.WrapAppError(err, &errs.ErrorOpts{})
	}

	os.WriteFile("parsed_result.json", js, 0644)
	return res, nil
}

type HeaderCache interface {
	// Assuming some interface, but not used in this implementation
}

type memHeaderCache struct {
	// Implementation if needed
}

func (this *ExcelParserService) parse(file []byte) ([]*app.ParseExcelResult, error) {
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
			this.log.Info("checking row for start", "sheet", sheet, "row", i, "nonEmptyCount", nonEmptyCount, "rowData", grid[i])
			if nonEmptyCount >= 3 {
				startRow = i
				break
			}
		}
		if startRow == -1 {
			this.log.Info("no start row found", "sheet", sheet)
			continue
		}
		this.log.Info("found start row", "sheet", sheet, "startRow", startRow)

		// Determine header rows using embeddings
		potentialHeaderRows := min(10, len(grid)-startRow)
		embeddings := make([][]float64, 0, potentialHeaderRows)
		rowTexts := make([]string, 0, potentialHeaderRows)
		this.log.Info("analyzing potential header rows", "sheet", sheet, "potentialHeaderRows", potentialHeaderRows)
		for j := 0; j < potentialHeaderRows; j++ {
			text := strings.Join(grid[startRow+j][:maxCol], " ")
			this.log.Info("potential header row", "sheet", sheet, "rowIndex", startRow+j, "text", text, "rowData", grid[startRow+j][:maxCol])
			if strings.TrimSpace(text) == "" {
				continue
			}
			rowTexts = append(rowTexts, text)
		}

		if len(rowTexts) < 2 {
			// Too few rows, assume no header or minimal
			headerRows := 0
			if len(rowTexts) == 1 {
				headerRows = 1
			}
			this.log.Info("too few rows for AI analysis, using fallback", "sheet", sheet, "rowTextsCount", len(rowTexts), "headerRows", headerRows)
			result := buildResult(grid, startRow, headerRows, maxCol)
			results = append(results, result)
			continue
		}

		// Get embeddings
		req := openai.EmbeddingNewParams{
			Model: openai.EmbeddingModelTextEmbedding3Small,
			Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: rowTexts},
		}
		resp, err := this.openaiClient.Embeddings.New(context.Background(), req)
		if err != nil {
			this.log.Error("failed to create embeddings", "error", err)
			// Fallback to heuristic: assume 1-2 header rows
			result := buildResult(grid, startRow, 1, maxCol)
			results = append(results, result)
			continue
		}

		for _, emb := range resp.Data {
			embeddings = append(embeddings, emb.Embedding)
		}

		this.log.Info("got embeddings", "sheet", sheet, "embeddingsCount", len(embeddings))

		// Find cutoff where similarity drops
		headerRows := 1

		// Look for transition from header-like to data-like content
		// Header rows typically have fewer filled cells and more structural text
		// Data rows typically have more filled cells with actual values
		for i := 1; i < len(embeddings); i++ {
			sim := cosineSimilarity(embeddings[i-1], embeddings[i])
			this.log.Info("similarity between rows", "sheet", sheet, "row1", i-1, "row2", i, "similarity", sim)

			// Check if current row looks like data (has enough non-empty cells that look like values)
			currentRowIndex := startRow + i
			if currentRowIndex < len(grid) {
				nonEmptyCount := 0
				hasDataLikeContent := false
				for _, cell := range grid[currentRowIndex] {
					if strings.TrimSpace(cell) != "" {
						nonEmptyCount++
						// Check if this looks like actual data (numbers, codes, etc.)
						if strings.Contains(cell, ".") || len(cell) > 10 ||
							(len(cell) > 3 && strings.ContainsAny(cell, "0123456789")) {
							hasDataLikeContent = true
						}
					}
				}

				this.log.Info("row analysis", "sheet", sheet, "rowIndex", currentRowIndex,
					"nonEmptyCount", nonEmptyCount, "hasDataLikeContent", hasDataLikeContent)

				// If we have enough filled cells and data-like content, this is probably where data starts
				if nonEmptyCount >= 3 && hasDataLikeContent {
					break
				}
			}

			headerRows = i + 1
		}

		// Cap at reasonable number
		if headerRows > 5 {
			headerRows = 3 // Reasonable fallback for complex headers
		}

		this.log.Info("determined header rows", "sheet", sheet, "headerRows", headerRows)

		result := buildResult(grid, startRow, headerRows, maxCol)

		// Log the final result
		this.log.Info("built result", "sheet", sheet, "header", result.Header, "rowsCount", len(result.Rows))

		results = append(results, result)
	}

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
		for j, cell := range rows[i] {
			grid[i][j] = cell
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

func buildResult(grid [][]string, startRow, headerRows, maxCol int) *app.ParseExcelResult {
	// Build header: lowest non-empty in header rows for each col
	header := make([]string, maxCol)
	for c := 0; c < maxCol; c++ {
		// Start from the top and go down, keeping the last (bottom-most) non-empty value
		for r := 0; r < headerRows; r++ {
			val := strings.TrimSpace(grid[startRow+r][c])
			if val != "" {
				header[c] = val
				// Don't break - continue to overwrite with lower values
			}
		}
		// If no value found, header[c] remains empty string (zero value)
	}

	// Log the final header construction
	// Note: this will be logged by the caller

	// Build rows: from startRow + headerRows to end, skip empty
	var rows [][]string
	for i := startRow + headerRows; i < len(grid); i++ {
		row := make([]string, maxCol)
		empty := true
		for j := 0; j < maxCol; j++ {
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

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
