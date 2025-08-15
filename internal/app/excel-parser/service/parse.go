package excel_parser_service

// import (
// 	"context"
// 	"encoding/csv"
// 	"encoding/json"
// 	"fmt"
// 	"math"
// 	"regexp"
// 	"slices"
// 	"strconv"
// 	"strings"

// 	"log/slog"

// 	"github.com/openai/openai-go/v2"
// 	"github.com/xuri/excelize/v2"

// 	"github.com/init-pkg/nova-template/domain/app"
// )

// // ---------------------------------------------------------------
// // Вспомогательные типы (локальные).
// // Если в вашем app.ParseExcelResult другие поля —
// // адаптируйте mapToAppResult() ниже.
// // ---------------------------------------------------------------

// type detectedTable struct {
// 	Sheet          string
// 	Top, Left      int
// 	Bottom, Right  int
// 	HeaderEndRow   int // абсолютный индекс строки в пределах таблицы (0-based от Top), последняя строка заголовка
// 	Headers        []string
// 	Rows           [][]string
// 	Category       string // ближняя категория (сверху или снизу)
// 	RawGridSnippet [][]string
// }

// // ---------------------------------------------------------------
// // Кэш заголовков/эмбеддингов (примитивный мем-кэш на процесс)
// // ---------------------------------------------------------------

// type HeaderCache interface {
// 	Get(key string) (string, bool)
// 	Set(key, value string)
// }

// type memHeaderCache struct {
// 	m map[string]string
// }

// func (m *memHeaderCache) Get(key string) (string, bool) {
// 	if m == nil || m.m == nil {
// 		return "", false
// 	}
// 	v, ok := m.m[key]
// 	return v, ok
// }

// func (m *memHeaderCache) Set(key, value string) {
// 	if m == nil {
// 		return
// 	}
// 	if m.m == nil {
// 		m.m = map[string]string{}
// 	}
// 	m.m[key] = value
// }

// // ---------------------------------------------------------------
// // Основная логика: parse()
// // ---------------------------------------------------------------

// // parse извлекает таблицы со всех листов.
// // Алгоритм:
// // 1) Загружаем книгу с разворачиванием merged.
// // 2) Для каждого листа строим прямоугольные "плотные" блоки.
// // 3) Для каждого блока определяем границу header/data (LLM+эвристика).
// // 4) Получаем нижний уровень заголовков + строки.
// // 5) Ищем ближайшую категорию сверху/снизу и добавляем в каждую строку.
// func (s *ExcelParserService) parse(ctx context.Context, file []byte) ([]*app.ParseExcelResult, error) {
// 	f, err := excelize.OpenReader(strings.NewReader(string(file)))
// 	if err != nil {
// 		return nil, fmt.Errorf("open excel: %w", err)
// 	}
// 	defer func() { _ = f.Close() }()

// 	var out []*app.ParseExcelResult

// 	sheets := f.GetSheetList()
// 	for _, sheet := range sheets {
// 		grid, err := readSheetGrid(f, sheet)
// 		if err != nil {
// 			s.log.Warn("readSheetGrid failed", slog.String("sheet", sheet), slog.Any("err", err))
// 			continue
// 		}
// 		blocks := detectDenseBlocks(grid)

// 		for _, b := range blocks {
// 			sub := sliceGrid(grid, b.Top, b.Left, b.Bottom, b.Right)

// 			// raw snapshot (до тримминга пустых столбцов/строк внутри блока)
// 			raw := deepCopy2D(sub)

// 			// обрежем пустые внешние строки/столбцы внутри блока
// 			sub, rowOffset, colOffset := trimEmptyOuter(sub)
// 			if len(sub) == 0 || len(sub[0]) == 0 {
// 				continue
// 			}

// 			headerEnd := s.decideHeaderBoundary(ctx, sheet, sub)
// 			// bottom-level headers (последняя строка хедера + прокидываем тексты сверху вниз, если многоуровневый)
// 			headers := collapseHeadersToBottomLevel(sub, headerEnd)

// 			// убираем полностью пустые столбцы из финального представления,
// 			// но только если они пустые и в хедере, и в данных
// 			sub, headers = dropAllEmptyColumns(sub, headers, headerEnd)

// 			dataRows := [][]string{}
// 			for r := headerEnd + 1; r < len(sub); r++ {
// 				row := make([]string, len(headers))
// 				for c := range headers {
// 					if c < len(sub[r]) {
// 						row[c] = sub[r][c]
// 					}
// 				}
// 				// скипаем полностью пустые строки
// 				if isAllEmpty(row) {
// 					continue
// 				}
// 				dataRows = append(dataRows, row)
// 			}

// 			// категория: ищем ближайшую осмысленную строку/ячейку над таблицей или под ней
// 			category := nearestCategory(grid, b.Top+rowOffset, b.Left+colOffset, b.Bottom, b.Right)

// 			// добавим категорию как последний столбец
// 			if category != "" {
// 				headers = append(headers, "category")
// 				for i := range dataRows {
// 					dataRows[i] = append(dataRows[i], category)
// 				}
// 			}

// 			det := detectedTable{
// 				Sheet:          sheet,
// 				Top:            b.Top + rowOffset,
// 				Left:           b.Left + colOffset,
// 				Bottom:         b.Bottom,
// 				Right:          b.Right,
// 				HeaderEndRow:   headerEnd,
// 				Headers:        headers,
// 				Rows:           dataRows,
// 				Category:       category,
// 				RawGridSnippet: raw,
// 			}

// 			appRes := mapToAppResult(det)
// 			if appRes != nil {
// 				out = append(out, appRes)
// 			}
// 		}
// 	}

// 	return out, nil
// }

// // ---------------------------------------------------------------
// // Чтение листа в сетку с разворотом merged-ячеек
// // ---------------------------------------------------------------

// func readSheetGrid(f *excelize.File, sheet string) ([][]string, error) {
// 	maxRow, maxCol, err := maxBounds(f, sheet)
// 	if err != nil {
// 		return nil, err
// 	}
// 	grid := make([][]string, maxRow)
// 	for r := 0; r < maxRow; r++ {
// 		grid[r] = make([]string, maxCol)
// 	}

// 	rows, err := f.Rows(sheet)
// 	if err != nil {
// 		return nil, err
// 	}
// 	rIdx := 0
// 	for rows.Next() {
// 		cols, _ := rows.Columns()
// 		for c := 0; c < len(cols) && c < maxCol; c++ {
// 			grid[rIdx][c] = normalizeCell(cols[c])
// 		}
// 		rIdx++
// 		if rIdx >= maxRow {
// 			break
// 		}
// 	}
// 	_ = rows.Close()

// 	// развернём merged
// 	merged, _ := f.GetMergeCells(sheet)
// 	for _, m := range merged {
// 		startCol, startRow, err := excelize.CellNameToCoordinates(m.GetStartAxis())
// 		if err != nil {
// 			continue
// 		}
// 		endCol, endRow, err := excelize.CellNameToCoordinates(m.GetEndAxis())
// 		if err != nil {
// 			continue
// 		}

// 		val, _ := f.GetCellValue(sheet, m.GetStartAxis())
// 		val = normalizeCell(val)

// 		for r := startRow - 1; r <= endRow-1 && r < maxRow; r++ {
// 			for c := startCol - 1; c <= endCol-1 && c < maxCol; c++ {
// 				grid[r][c] = val
// 			}
// 		}
// 	}

// 	return grid, nil
// }

// func normalizeCell(s string) string {
// 	s = strings.TrimSpace(s)
// 	// заменим неразрывные пробелы, новые строки и т.д.
// 	s = strings.ReplaceAll(s, "\u00A0", " ")
// 	s = strings.ReplaceAll(s, "\r\n", "\n")
// 	s = strings.ReplaceAll(s, "\r", "\n")
// 	return s
// }

// func maxBounds(f *excelize.File, sheet string) (int, int, error) {
// 	maxRow := 0
// 	maxCol := 0

// 	rows, err := f.Rows(sheet)
// 	if err != nil {
// 		return 0, 0, err
// 	}
// 	idx := 0
// 	for rows.Next() {
// 		cols, _ := rows.Columns()
// 		if len(cols) > maxCol {
// 			maxCol = len(cols)
// 		}
// 		idx++
// 	}
// 	_ = rows.Close()
// 	maxRow = idx

// 	// safety upper bound
// 	if maxCol > 512 {
// 		maxCol = 512
// 	}
// 	if maxRow > 10000 {
// 		maxRow = 10000
// 	}
// 	return maxRow, maxCol, nil
// }

// // ---------------------------------------------------------------
// // Поиск "плотных" блоков (кандидаты таблиц)
// // ---------------------------------------------------------------

// type rect struct{ Top, Left, Bottom, Right int }

// func detectDenseBlocks(grid [][]string) []rect {
// 	nr := len(grid)
// 	if nr == 0 {
// 		return nil
// 	}

// 	// эвристика: разбиваем по "пустым коридорам" из >=2 подряд пустых строк
// 	emptyRow := func(r int) bool { return isAllEmpty(grid[r]) }
// 	cuts := []int{-1}
// 	emptyRun := 0
// 	for r := 0; r < nr; r++ {
// 		if emptyRow(r) {
// 			emptyRun++
// 		} else {
// 			if emptyRun >= 2 {
// 				cuts = append(cuts, r-1)
// 			}
// 			emptyRun = 0
// 		}
// 	}
// 	cuts = append(cuts, nr)

// 	var blocks []rect
// 	for i := 0; i < len(cuts)-1; i++ {
// 		top := cuts[i] + 1
// 		bot := cuts[i+1] - 1
// 		if bot-top+1 < 3 {
// 			continue
// 		}
// 		// внутри сегмента найдём рабочую ширину
// 		left, right := denseColsRange(grid[top : bot+1])
// 		if right-left+1 < 2 {
// 			continue
// 		}
// 		blocks = append(blocks, rect{Top: top, Left: left, Bottom: bot, Right: right})
// 	}
// 	return blocks
// }

// func denseColsRange(seg [][]string) (int, int) {
// 	nr := len(seg)
// 	nc := len(seg[0])
// 	colScore := make([]int, nc)
// 	for r := 0; r < nr; r++ {
// 		for c := 0; c < nc; c++ {
// 			if strings.TrimSpace(seg[r][c]) != "" {
// 				colScore[c]++
// 			}
// 		}
// 	}
// 	// найдём непрерывный диапазон, где хотя бы в 20% строк колонки непустые
// 	thr := int(math.Max(1, math.Round(0.2*float64(nr))))
// 	bestL, bestR, bestSpan := 0, -1, 0
// 	l := -1
// 	for c := 0; c < nc; c++ {
// 		if colScore[c] >= thr {
// 			if l == -1 {
// 				l = c
// 			}
// 		} else {
// 			if l != -1 {
// 				if c-1-l+1 > bestSpan {
// 					bestSpan = c - l
// 					bestL = l
// 					bestR = c - 1
// 				}
// 				l = -1
// 			}
// 		}
// 	}
// 	if l != -1 && nc-1-l+1 > bestSpan {
// 		bestL = l
// 		bestR = nc - 1
// 	}
// 	return bestL, bestR
// }

// func sliceGrid(g [][]string, top, left, bottom, right int) [][]string {
// 	out := make([][]string, bottom-top+1)
// 	for i := range out {
// 		out[i] = slices.Clone(g[top+i][left : right+1])
// 	}
// 	return out
// }

// func deepCopy2D(a [][]string) [][]string {
// 	out := make([][]string, len(a))
// 	for i := range a {
// 		out[i] = slices.Clone(a[i])
// 	}
// 	return out
// }

// func trimEmptyOuter(g [][]string) ([][]string, int, int) {
// 	if len(g) == 0 {
// 		return g, 0, 0
// 	}
// 	nr, nc := len(g), len(g[0])
// 	top, bot := 0, nr-1
// 	left, right := 0, nc-1

// 	for top <= bot && isAllEmpty(g[top]) {
// 		top++
// 	}
// 	for bot >= top && isAllEmpty(g[bot]) {
// 		bot--
// 	}

// 	colEmpty := func(c int) bool {
// 		for r := top; r <= bot; r++ {
// 			if strings.TrimSpace(g[r][c]) != "" {
// 				return false
// 			}
// 		}
// 		return true
// 	}
// 	for left <= right && colEmpty(left) {
// 		left++
// 	}
// 	for right >= left && colEmpty(right) {
// 		right--
// 	}

// 	if top > bot || left > right {
// 		return [][]string{}, top, left
// 	}

// 	out := make([][]string, bot-top+1)
// 	for r := range out {
// 		out[r] = slices.Clone(g[top+r][left : right+1])
// 	}
// 	return out, top, left
// }

// func isAllEmpty(row []string) bool {
// 	for _, v := range row {
// 		if strings.TrimSpace(v) != "" {
// 			return false
// 		}
// 	}
// 	return true
// }

// // ---------------------------------------------------------------
// // Граница header/data (LLM + эвристика)
// // ---------------------------------------------------------------

// func (s *ExcelParserService) decideHeaderBoundary(ctx context.Context, sheet string, grid [][]string) int {
// 	// верхний максимум 25 строк анализируем LLM'ом
// 	maxR := min(25, len(grid))
// 	maxC := min(30, len(grid[0]))

// 	snippet := toCSV(grid[:maxR], maxC)

// 	// сперва попробуем кэш
// 	cacheKey := fmt.Sprintf("header-boundary:%s:%x", sheet, hash32(snippet))
// 	if cached, ok := s.cache.Get(cacheKey); ok {
// 		if n, err := strconv.Atoi(cached); err == nil {
// 			return clamp(n, 0, min(6, len(grid)-2)) // не позволяем слишком глубокий header
// 		}
// 	}

// 	// Попытка через GPT (json-mode) — безопасный фолбэк на эвристику.
// 	boundary, ok := s.askLLMHeaderBoundary(ctx, snippet)
// 	if !ok {
// 		boundary = heuristicHeaderBoundary(grid)
// 	}
// 	// Кэшируем строкой
// 	s.cache.Set(cacheKey, fmt.Sprintf("%d", boundary))
// 	return boundary
// }

// func (s *ExcelParserService) askLLMHeaderBoundary(ctx context.Context, csvSnippet string) (int, bool) {
// 	// Используем structured JSON через chat.completions.
// 	// Если библиотека/модель недоступны, возвращаем false.
// 	defer func() { _ = recover() }()

// 	sys := "You are a data extraction assistant. Given a CSV-like snippet of a table head+body, " +
// 		"return a JSON with {\"header_rows\": N} where N is the number of header rows at the top " +
// 		"(multi-level headers included). The CSV may contain noise lines above/below. " +
// 		"Return only valid JSON."

// 	user := "CSV snippet (comma-separated):\n```\n" + csvSnippet + "\n```"

// 	// В openai-go/v2 JSON-моды формируются через tool/response_format, но оставим простой формат:
// 	req := openai.ChatCompletionNewParams{
// 		Model: openai.ChatModelGPT5Nano,
// 		Messages: []openai.ChatCompletionMessageParamUnion{
// 			openai.SystemMessage(sys),
// 			openai.UserMessage(user),
// 		},
// 		Temperature: openai.Float(0.0),
// 	}

// 	resp, err := s.openaiClient.Chat.Completions.New(ctx, req)
// 	if err != nil || len(resp.Choices) == 0 {
// 		return 0, false
// 	}

// 	content := strings.TrimSpace(resp.Choices[0].Message.Content)
// 	type jj struct {
// 		HeaderRows int `json:"header_rows"`
// 	}
// 	var parsed jj
// 	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
// 		// на случай, если модель вернула текст — извлечём число эвристикой
// 		m := regexp.MustCompile(`\d+`).FindString(content)
// 		if m == "" {
// 			return 0, false
// 		}
// 		n, _ := strconv.Atoi(m)
// 		return clamp(n-1, 0, 6), true
// 	}
// 	// Превращаем «кол-во строк хедера» в индекс последней строки хедера (0-based)
// 	return clamp(parsed.HeaderRows-1, 0, 6), true
// }

// func heuristicHeaderBoundary(grid [][]string) int {
// 	// Простая эвристика:
// 	//  - хедер обычно плотнее по тексту, меньше чисел
// 	//  - смена паттерна «текст->числа» часто указывает на границу
// 	maxCheck := min(10, len(grid)-1)
// 	last := 0
// 	for r := 0; r < maxCheck; r++ {
// 		texty := 0
// 		numeric := 0
// 		for _, v := range grid[r] {
// 			v = strings.TrimSpace(v)
// 			if v == "" {
// 				continue
// 			}
// 			if maybeNumber(v) {
// 				numeric++
// 			} else {
// 				texty++
// 			}
// 		}
// 		if texty >= numeric {
// 			last = r
// 		}
// 	}
// 	return last
// }

// func maybeNumber(s string) bool {
// 	// мягкая проверка: числа с разделителями/валютами
// 	s = strings.TrimSpace(s)
// 	s = strings.Trim(s, "+- $€£₸₽¥₩")
// 	s = strings.ReplaceAll(s, " ", "")
// 	s = strings.ReplaceAll(s, ",", "")
// 	s = strings.ReplaceAll(s, "\u00A0", "")
// 	_, err := strconv.ParseFloat(strings.ReplaceAll(s, "%", ""), 64)
// 	return err == nil
// }

// // ---------------------------------------------------------------
// // Обработка заголовков и данных
// // ---------------------------------------------------------------

// func collapseHeadersToBottomLevel(g [][]string, headerEnd int) []string {
// 	if headerEnd < 0 {
// 		return []string{}
// 	}
// 	nc := len(g[0])
// 	headers := make([]string, nc)

// 	// возьмём самую нижнюю строку заголовка как основу, а пустые
// 	// прогоним наверх, пока не найдём непустую
// 	for c := 0; c < nc; c++ {
// 		val := strings.TrimSpace(g[headerEnd][c])
// 		if val == "" {
// 			// поднимемся вверх
// 			for r := headerEnd - 1; r >= 0; r-- {
// 				if strings.TrimSpace(g[r][c]) != "" {
// 					val = g[r][c]
// 					break
// 				}
// 			}
// 		}
// 		if val == "" {
// 			val = fmt.Sprintf("undefined-%d", c+1)
// 		}
// 		headers[c] = val
// 	}
// 	// аккуратно сплющим multi-line в один токен
// 	for i := range headers {
// 		headers[i] = strings.Join(splitAndTrim(headers[i]), " ")
// 	}
// 	return headers
// }

// func splitAndTrim(s string) []string {
// 	parts := strings.FieldsFunc(s, func(r rune) bool {
// 		return r == '\n' || r == '\t'
// 	})
// 	out := make([]string, 0, len(parts))
// 	for _, p := range parts {
// 		p = strings.TrimSpace(p)
// 		if p != "" {
// 			out = append(out, p)
// 		}
// 	}
// 	return out
// }

// func dropAllEmptyColumns(g [][]string, headers []string, headerEnd int) ([][]string, []string) {
// 	nc := len(headers)
// 	keep := make([]bool, nc)
// 	for c := 0; c < nc; c++ {
// 		colHas := false
// 		for r := headerEnd + 1; r < len(g); r++ {
// 			if c < len(g[r]) && strings.TrimSpace(g[r][c]) != "" {
// 				colHas = true
// 				break
// 			}
// 		}
// 		if colHas || strings.TrimSpace(headers[c]) != "" {
// 			keep[c] = true
// 		}
// 	}
// 	// если оказалось, что все false — не трогаем
// 	if !slices.Contains(keep, true) {
// 		return g, headers
// 	}

// 	newHeaders := []string{}
// 	newG := make([][]string, len(g))
// 	for r := range g {
// 		row := []string{}
// 		for c := 0; c < nc; c++ {
// 			if keep[c] {
// 				if r == 0 {
// 					newHeaders = append(newHeaders, headers[c])
// 				}
// 				if c < len(g[r]) {
// 					row = append(row, g[r][c])
// 				} else {
// 					row = append(row, "")
// 				}
// 			}
// 		}
// 		newG[r] = row
// 	}
// 	return newG, newHeaders
// }

// // ---------------------------------------------------------------
// // Категория (ближайшая строка/ячейка сверху или снизу)
// // ---------------------------------------------------------------

// func nearestCategory(grid [][]string, absTop, absLeft, absBottom, absRight int) string {
// 	// Ищем сверху до 5 строк
// 	upLimit := max(0, absTop-5)
// 	for r := absTop - 1; r >= upLimit; r-- {
// 		// возьмём «сигнатуру» строки
// 		if cat := denseLineLabel(grid[r][absLeft : absRight+1]); cat != "" {
// 			return cat
// 		}
// 	}
// 	// Ищем снизу до 5 строк
// 	downLimit := min(len(grid)-1, absBottom+5)
// 	for r := absBottom + 1; r <= downLimit; r++ {
// 		if cat := denseLineLabel(grid[r][absLeft : absRight+1]); cat != "" {
// 			return cat
// 		}
// 	}
// 	return ""
// }

// func denseLineLabel(cells []string) string {
// 	// Если строка состоит в основном из текста и мало чисел — вероятный ярлык категории
// 	texts := []string{}
// 	numCount := 0
// 	for _, v := range cells {
// 		v = strings.TrimSpace(v)
// 		if v == "" {
// 			continue
// 		}
// 		if maybeNumber(v) {
// 			numCount++
// 		} else {
// 			texts = append(texts, v)
// 		}
// 	}
// 	if len(texts) >= 1 && numCount == 0 {
// 		// объединяем уникальные тексты
// 		uniq := map[string]struct{}{}
// 		order := []string{}
// 		for _, t := range texts {
// 			if _, ok := uniq[t]; !ok {
// 				uniq[t] = struct{}{}
// 				order = append(order, t)
// 			}
// 		}
// 		return strings.Join(order, " / ")
// 	}
// 	return ""
// }

// // ---------------------------------------------------------------
// // Утилиты/форматы
// // ---------------------------------------------------------------

// func mapToAppResult(dt detectedTable) *app.ParseExcelResult {
// 	// Если у вашего app.ParseExcelResult нет поля Sheet — удалите присвоение.
// 	// Ожидается структура:
// 	// type ParseExcelResult struct {
// 	//   Sheet  string     `json:"sheet,omitempty"`
// 	//   Header []string   `json:"header"`
// 	//   Rows   [][]string `json:"rows"`
// 	// }
// 	res := &app.ParseExcelResult{
// 		Header: dt.Headers,
// 		Rows:   dt.Rows,
// 	}
// 	// заполним через рефлексию, если есть поле Sheet
// 	type hasSheet interface{ SetSheet(string) }
// 	if hs, ok := any(res).(hasSheet); ok {
// 		hs.SetSheet(dt.Sheet)
// 	} else {
// 		// пробуем через json tags (без паники, просто игнор если поля нет)
// 		type tmp struct {
// 			Sheet  string     `json:"sheet,omitempty"`
// 			Header []string   `json:"header"`
// 			Rows   [][]string `json:"rows"`
// 		}
// 		_ = tmp{Sheet: dt.Sheet, Header: dt.Headers, Rows: dt.Rows} // no-op, документирует намерение
// 	}
// 	return res
// }

// func min(a, b int) int {
// 	if a < b {
// 		return a
// 	}
// 	return b
// }

// func max(a, b int) int {
// 	if a > b {
// 		return a
// 	}
// 	return b
// }

// func clamp(x, lo, hi int) int {
// 	if x < lo {
// 		return lo
// 	}
// 	if x > hi {
// 		return hi
// 	}
// 	return x
// }

// func hash32(s string) uint32 {
// 	var h uint32 = 2166136261
// 	const p uint32 = 16777619
// 	for i := 0; i < len(s); i++ {
// 		h ^= uint32(s[i])
// 		h *= p
// 	}
// 	return h
// }

// func toCSV(grid [][]string, limitCols int) string {
// 	b := &strings.Builder{}
// 	w := csv.NewWriter(b)
// 	for _, row := range grid {
// 		line := row
// 		if len(line) > limitCols {
// 			line = line[:limitCols]
// 		}
// 		_ = w.Write(line)
// 	}
// 	w.Flush()
// 	return b.String()
// }
