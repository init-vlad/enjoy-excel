package excel_parser_service

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/init-pkg/nova-template/domain/app"
	"github.com/xuri/excelize/v2"
)

func (s *ExcelParserService) parse(file []byte) (*app.ParseExcelResult, error) {
	// 1) Открываем .xlsx из памяти
	f, err := excelize.OpenReader(bytes.NewReader(file))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}

	type candidate struct {
		sheet string
		rect  Rect
		score float64 // плотность * регулярность
	}

	var best *candidate

	// 2) На каждом листе строим маску занятости и ищем лучший прямоугольник таблицы
	for _, sh := range sheets {
		rows, err := f.GetRows(sh, excelize.Options{RawCellValue: true})
		if err != nil {
			continue
		}
		if len(rows) == 0 {
			continue
		}

		rMax, cMax := usedBounds(rows)

		// Маска заполненности (с учётом merge → помечаем весь merge-диапазон как "занято")
		mask := makeMask(rows, rMax, cMax)

		// Учитываем merge-диапазоны
		if merges, err := f.GetMergeCells(sh); err == nil {
			for _, mg := range merges {
				sr, sc := refToRC(mg.GetStartAxis())
				er, ec := refToRC(mg.GetEndAxis())
				if sr <= 0 || sc <= 0 || er <= 0 || ec <= 0 {
					continue
				}
				for r := sr - 1; r <= min(er-1, rMax-1); r++ {
					for c := sc - 1; c <= min(ec-1, cMax-1); c++ {
						if sr-1 < len(rows) && sc-1 < len(rows[sr-1]) && rows[sr-1][sc-1] != "" {
							mask[r][c] = true
						}
					}
				}
			}
		}

		// (Необязательно) Попытка игнора картинок: excelize не даёт точного bbox,
		// поэтому специально ничего не вычитаем; маска по тексту/числам и так отфильтрует плавающий мусор.

		// Находим top-k прямоугольников из единиц
		rects := largestRectangles(mask, 3)

		// Оценим кандидатов: плотность + регулярность строк
		for _, r := range rects {
			if r.Empty() {
				continue
			}
			density := rectDensity(mask, r)
			if density < 0.55 { // слегка консервативный порог
				continue
			}
			reg := rowRegularity(rows, r)
			sc := density*0.7 + reg*0.3
			if best == nil || sc > best.score {
				rc := r // копия
				best = &candidate{sheet: sh, rect: rc, score: sc}
			}
		}
	}

	if best == nil || best.rect.Empty() {
		return nil, fmt.Errorf("failed to locate table block")
	}

	// 3) Режем блок, детектируем глубину хедера, собираем колонки и строки
	rows, _ := f.GetRows(best.sheet, excelize.Options{RawCellValue: true})
	_, maxCols := usedBounds(rows)

	// предварительный блок по старому прямоугольнику
	preBlock := sliceBlock(rows, best.rect)
	if len(preBlock) == 0 {
		return nil, fmt.Errorf("empty block")
	}

	// прикидываем глубину хедера на предварительном блоке
	preH := detectHeaderDepth(preBlock)

	// расширим прямоугольник по окну первых строк данных и дотянем вниз
	refined := expandRectHoriz(rows, best.rect, preH, maxCols)
	refined = extendRectDown(rows, refined)

	// теперь работаем уже с уточнённым прямоугольником
	block := sliceBlock(rows, refined)
	if len(block) == 0 {
		return nil, fmt.Errorf("empty refined block")
	}

	h := detectHeaderDepth(block)
	header := buildHeader(block, h)
	data := block[h:]
	idxs := columnsToKeep(header, data)
	header, data = projectColumns(header, data, idxs)

	// Нормализуем ширину (некоторые строки могут быть короче)
	maxW := 0
	for _, r := range data {
		if len(r) > maxW {
			maxW = len(r)
		}
	}
	if len(header) < maxW {
		// добьём хедер до длины данных
		delta := maxW - len(header)
		header = append(header, make([]string, delta)...)
	}
	for i := range data {
		if len(data[i]) < maxW {
			d := make([]string, maxW-len(data[i]))
			data[i] = append(data[i], d...)
		}
	}

	// fin: отрезаем полностью пустые строки снизу (страховка)
	data = trimEmptyRows(data)

	res := &app.ParseExcelResult{
		Header: header,
		Rows:   data,
	}
	return res, nil
}

// ---- Геометрия и маски ----

type Rect struct{ R1, C1, R2, C2 int } // включительно

func (r Rect) Empty() bool { return r.R2 < r.R1 || r.C2 < r.C1 }
func (r Rect) Height() int { return r.R2 - r.R1 + 1 }
func (r Rect) Width() int  { return r.C2 - r.C1 + 1 }

func usedBounds(rows [][]string) (int, int) {
	rMax := len(rows)
	cMax := 0
	for _, r := range rows {
		if len(r) > cMax {
			cMax = len(r)
		}
	}
	return rMax, cMax
}

func makeMask(rows [][]string, rMax, cMax int) [][]bool {
	mask := make([][]bool, rMax)
	for i := range mask {
		mask[i] = make([]bool, cMax)
		for j := 0; j < cMax; j++ {
			var v string
			if j < len(rows[i]) {
				v = strings.TrimSpace(rows[i][j])
			}
			mask[i][j] = isSignalCell(v)
		}
	}
	return mask
}

func isSignalCell(v string) bool {
	if v == "" {
		return false
	}
	// Отсекаем одиночные мусорные символы
	if len([]rune(v)) <= 1 {
		r := []rune(v)[0]
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}

func rectDensity(mask [][]bool, r Rect) float64 {
	var ones, area int
	for i := r.R1; i <= r.R2; i++ {
		row := mask[i]
		for j := r.C1; j <= r.C2; j++ {
			area++
			if row[j] {
				ones++
			}
		}
	}
	if area == 0 {
		return 0
	}
	return float64(ones) / float64(area)
}

// Оценка регулярности: SD по количеству непустых на строку → нормируем и инвертируем
func rowRegularity(rows [][]string, r Rect) float64 {
	cnt := make([]int, 0, r.Height())
	for i := r.R1; i <= r.R2; i++ {
		row := rows[i]
		n := 0
		for j := r.C1; j <= r.C2; j++ {
			if j < len(row) && strings.TrimSpace(row[j]) != "" {
				n++
			}
		}
		cnt = append(cnt, n)
	}
	mean := 0.0
	for _, v := range cnt {
		mean += float64(v)
	}
	mean /= float64(len(cnt))
	variance := 0.0
	for _, v := range cnt {
		d := float64(v) - mean
		variance += d * d
	}
	variance /= float64(len(cnt))
	sd := math.Sqrt(variance)
	// 0 → идеально, 1+ → хуже; приведем к [0..1]
	norm := 1.0 / (1.0 + sd)
	return norm
}

// Largest rectangle in binary matrix (возвращаем top-k по площади)
func largestRectangles(mask [][]bool, k int) []Rect {
	h := len(mask)
	if h == 0 {
		return nil
	}
	w := len(mask[0])
	heights := make([]int, w)
	type candidate struct {
		rect Rect
		area int
	}
	var cands []candidate

	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			if mask[i][j] {
				heights[j]++
			} else {
				heights[j] = 0
			}
		}
		for _, cand := range largestRectInHistogram(heights, i) {
			cands = append(cands, cand)
		}
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].area > cands[j].area })
	if len(cands) > k {
		cands = cands[:k]
	}
	out := make([]Rect, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.rect)
	}
	return out
}

func largestRectInHistogram(heights []int, row int) []struct {
	rect Rect
	area int
} {
	type stEl struct{ idx, h int }
	var st []stEl
	var res []struct {
		rect Rect
		area int
	}

	for i := 0; i <= len(heights); i++ {
		curH := 0
		if i < len(heights) {
			curH = heights[i]
		}
		start := i
		for len(st) > 0 && st[len(st)-1].h > curH {
			top := st[len(st)-1]
			st = st[:len(st)-1]
			width := i
			if len(st) > 0 {
				width = i - st[len(st)-1].idx - 1
				start = st[len(st)-1].idx + 1
			} else {
				start = 0
			}
			if top.h > 0 && width > 0 {
				r := Rect{
					R1: row - top.h + 1,
					R2: row,
					C1: start,
					C2: i - 1,
				}
				res = append(res, struct {
					rect Rect
					area int
				}{rect: r, area: top.h * width})
			}
		}
		st = append(st, stEl{idx: i, h: curH})
	}
	return res
}

// ---- Вырез блока и предобработка ----

func sliceBlock(rows [][]string, r Rect) [][]string {
	out := make([][]string, 0, r.Height())
	for i := r.R1; i <= r.R2; i++ {
		line := make([]string, 0, r.Width())
		for j := r.C1; j <= r.C2; j++ {
			var v string
			if i < len(rows) && j < len(rows[i]) {
				v = strings.TrimSpace(rows[i][j])
			}
			line = append(line, v)
		}
		out = append(out, line)
	}
	return out
}

func trimEmptyRows(data [][]string) [][]string {
	isEmpty := func(r []string) bool {
		for _, v := range r {
			if strings.TrimSpace(v) != "" {
				return false
			}
		}
		return true
	}
	// снизу
	for len(data) > 0 && isEmpty(data[len(data)-1]) {
		data = data[:len(data)-1]
	}
	// сверху (на всякий)
	for len(data) > 0 && isEmpty(data[0]) {
		data = data[1:]
	}
	return data
}

// ---- Детекция глубины хедера ----

var reNumber = regexp.MustCompile(`^\s*[\+\-]?\d{1,3}([.,]\d{3})*([.,]\d+)?\s*$`)
var reMoney = regexp.MustCompile(`(?i)(₸|тг|kzt|руб|₽|usd|eur|%|с ндс|без ндс)`)
var reDate = regexp.MustCompile(`^\s*\d{1,2}[\./-]\d{1,2}[\./-]\d{2,4}\s*$`)

var headerDict = []string{
	"артикул", "sku", "код", "наименование", "название", "товар", "единица", "ед.изм",
	"единицы", "бренд", "категория", "цена", "розница", "опт", "количество", "остаток",
	"шт", "вес", "объем", "ед", "упак", "упаковка", "баркод", "штрихкод", "поставщик",
}

// --- NEW: оценка "похожести на хедер" для строки
func headerRowLikelihood(row []string) float64 {
	total, dictHits, numbers := 0, 0, 0
	for _, v := range row {
		vv := strings.ToLower(strings.TrimSpace(v))
		if vv == "" {
			continue
		}
		total++
		for _, h := range headerDict {
			if strings.Contains(vv, h) {
				dictHits++
				break
			}
		}
		if reNumber.MatchString(vv) || reDate.MatchString(vv) {
			numbers++
		}
	}
	if total == 0 {
		return 0
	}
	// больше совпадений со словарём и меньше чисел → больше шанс, что это хедер
	return (float64(dictHits)/float64(total))*0.7 + (1.0-float64(numbers)/float64(total))*0.3
}

// --- FIXED: детекция глубины хедера (с ограничением и рескорингом)
func detectHeaderDepth(block [][]string) int {
	n := len(block)
	if n == 0 {
		return 0
	}
	maxTry := min(6, n)

	// 1) Посчитаем "data-score" по строкам
	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		var next []string
		if i+1 < n {
			next = block[i+1]
		}
		scores[i] = rowDataScore(block[i], i+1 < n, next)
	}

	// 2) Найдём первый "забег" из >=2 подряд строк, похожих на данные
	thr := 0.55
	run := 0
	dataStart := -1
	for i := 0; i < min(n, 20); i++ {
		if scores[i] >= thr {
			run++
			if run >= 2 {
				dataStart = i - run + 1 // индекс начала данных
				break
			}
		} else {
			run = 0
		}
	}

	// 3) Если не нашли — fallback как раньше (перебор 1..maxTry)
	if dataStart == -1 {
		bestH, bestScore := 1, -1.0
		for h := 1; h <= maxTry; h++ {
			if h >= n {
				break
			}
			// комбинированный скор: стабильность типов в данных + "хедерность" верхних строк
			typStability := columnTypingStability(block[h:])
			hdrLik := 0.0
			for i := 0; i < h; i++ {
				hdrLik += headerRowLikelihood(block[i])
			}
			// веса: делаем акцент на корректной сегментации данных
			score := 0.75*typStability + 0.25*(hdrLik/float64(h))
			if score > bestScore {
				bestScore, bestH = score, h
			}
		}
		return bestH
	}

	// 4) Нашли старт данных: не верим сразу большому h.
	//    Ограничим хедер максимум 3 строками (часто хватает) и подберём лучшее h по метрике.
	maxH := min(3, min(dataStart, maxTry))
	if maxH <= 0 {
		return 1
	}

	bestH, bestScore := 1, -1.0
	for h := 1; h <= maxH; h++ {
		typStability := columnTypingStability(block[h:])
		hdrLik := 0.0
		for i := 0; i < h; i++ {
			hdrLik += headerRowLikelihood(block[i])
		}
		score := 0.75*typStability + 0.25*(hdrLik/float64(h))
		if score > bestScore {
			bestScore, bestH = score, h
		}
	}
	return bestH
}

func rowDataScore(row []string, hasNext bool, next []string) float64 {
	if len(row) == 0 {
		return 0
	}
	total := 0
	num := 0
	dictHits := 0
	unitHits := 0
	for _, v := range row {
		vv := strings.ToLower(strings.TrimSpace(v))
		if vv == "" {
			continue
		}
		total++
		if reNumber.MatchString(vv) || reDate.MatchString(vv) {
			num++
		}
		if reMoney.MatchString(vv) {
			unitHits++
		}
		for _, h := range headerDict {
			if strings.Contains(vv, h) {
				dictHits++
				break
			}
		}
	}
	numRatio := 0.0
	if total > 0 {
		numRatio = float64(num) / float64(total)
	}

	simNext := 0.0
	if hasNext && next != nil {
		simNext = jaccardRow(row, next)
	}

	// чем больше dictHits/unitHits — тем больше это похоже на хедер (понижаем скор)
	penalty := 0.0
	if total > 0 {
		penalty = float64(dictHits)/float64(total)*0.4 + float64(unitHits)/float64(total)*0.2
	}

	score := 0.45*numRatio + 0.40*simNext - penalty
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

func jaccardRow(a, b []string) float64 {
	tok := func(r []string) map[string]struct{} {
		m := make(map[string]struct{})
		for _, v := range r {
			for _, t := range tokenize(v) {
				m[t] = struct{}{}
			}
		}
		return m
	}
	ma, mb := tok(a), tok(b)
	if len(ma) == 0 && len(mb) == 0 {
		return 0
	}
	inter := 0
	for t := range ma {
		if _, ok := mb[t]; ok {
			inter++
		}
	}
	union := len(ma) + len(mb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, ";", " ")
	s = strings.ReplaceAll(s, "/", " ")
	s = strings.ReplaceAll(s, "\\", " ")
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimFunc(f, func(r rune) bool {
			return !(unicode.IsDigit(r) || unicode.IsLetter(r))
		})
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func columnTypingStability(data [][]string) float64 {
	if len(data) == 0 {
		return 0
	}
	maxW := 0
	for _, r := range data {
		if len(r) > maxW {
			maxW = len(r)
		}
	}
	if maxW == 0 {
		return 0
	}
	typeCounts := make([]map[string]int, maxW)
	for i := 0; i < maxW; i++ {
		typeCounts[i] = make(map[string]int)
	}
	sample := min(60, len(data))
	for i := 0; i < sample; i++ {
		row := data[i]
		for j := 0; j < maxW; j++ {
			var v string
			if j < len(row) {
				v = row[j]
			}
			t := inferCellType(v)
			typeCounts[j][t]++
		}
	}
	// Оцениваем среднюю "сконцентрированность" типов по колонкам (peak / total)
	total := 0.0
	for j := 0; j < maxW; j++ {
		col := typeCounts[j]
		sum := 0
		peak := 0
		for _, c := range col {
			sum += c
			if c > peak {
				peak = c
			}
		}
		if sum > 0 {
			total += float64(peak) / float64(sum)
		}
	}
	return total / float64(maxW) // 0..1
}

func inferCellType(v string) string {
	vv := strings.ToLower(strings.TrimSpace(v))
	switch {
	case vv == "":
		return "empty"
	case reNumber.MatchString(vv):
		return "number"
	case reDate.MatchString(vv):
		return "date"
	case reMoney.MatchString(vv):
		return "money"
	case looksLikeSKU(vv):
		return "sku"
	default:
		return "text"
	}
}

func looksLikeSKU(v string) bool {
	// простая эвристика: длинная без пробелов, буквы/цифры/-,_/ — похоже на артикул
	if strings.ContainsAny(v, " \t") {
		return false
	}
	r := []rune(v)
	if len(r) < 5 {
		return false
	}
	okChars := 0
	for _, ch := range r {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '_' || ch == '/' {
			okChars++
		}
	}
	return float64(okChars)/float64(len(r)) > 0.8
}

// ---- Построение хедера ----

func buildHeader(block [][]string, h int) []string {
	if h <= 0 {
		h = 1
	}
	h = min(h, len(block))

	// ширина берём по всему блоку — чтобы увидеть правый хвост
	maxW := 0
	for _, r := range block {
		if len(r) > maxW {
			maxW = len(r)
		}
	}

	header := make([]string, maxW)
	for j := 0; j < maxW; j++ {
		var parts []string
		for i := 0; i < h; i++ {
			if j < len(block[i]) && strings.TrimSpace(block[i][j]) != "" {
				parts = append(parts, strings.TrimSpace(block[i][j]))
			}
		}
		if len(parts) == 0 {
			// посмотрим, есть ли данные под колонкой
			hasData := false
			for r := h; r < len(block); r++ {
				if j < len(block[r]) && strings.TrimSpace(block[r][j]) != "" {
					hasData = true
					break
				}
			}
			if hasData {
				header[j] = fmt.Sprintf("undefined_%d", j+1)
			} else {
				header[j] = fmt.Sprintf("col_%d", j+1) // временное имя (скорее всего колонка удалится)
			}
		} else {
			header[j] = normalizeHeader(strings.Join(parts, " / "))
		}
	}
	return header
}

func normalizeHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	reSpaces := regexp.MustCompile(`\s+`)
	s = reSpaces.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		s = "col"
	}
	return s
}

// ---- Утилиты ----

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func refToRC(ref string) (int, int) {
	// "B3" -> (3, 2)
	re := regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)
	m := re.FindStringSubmatch(ref)
	if m == nil {
		return 0, 0
	}
	col := titleToNumber(m[1])
	row := 0
	fmt.Sscanf(m[2], "%d", &row)
	return row, col
}

func titleToNumber(s string) int {
	s = strings.ToUpper(s)
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*26 + int(s[i]-'A') + 1
	}
	return n
}

// возвращает список индексов колонок, которые нужно сохранить
func columnsToKeep(header []string, data [][]string) []int {
	maxW := len(header)
	W := min(200, len(data)) // окно по данным
	keep := make([]bool, maxW)

	for j := 0; j < maxW; j++ {
		// признак "заголовок задан"
		hasHeader := header[j] != "" && !strings.HasPrefix(header[j], "col_")
		// считаем непустые в данных
		nonEmpty := 0
		for i := 0; i < W; i++ {
			if j < len(data[i]) && strings.TrimSpace(data[i][j]) != "" {
				nonEmpty++
				if nonEmpty >= 3 { // ранний выход: точно есть данные
					break
				}
			}
		}
		// порог для "почти пустой" колонки: ≤2 непустых на первых W строках
		if hasHeader || nonEmpty >= 3 {
			keep[j] = true
		} else {
			keep[j] = false // хвост мерджа/мусор — выкидываем
		}
	}

	// сформируем список индексов
	out := make([]int, 0, maxW)
	for j := 0; j < maxW; j++ {
		if keep[j] {
			out = append(out, j)
		}
	}
	return out
}

// применяем список индексов к header и data
func projectColumns(header []string, data [][]string, idxs []int) ([]string, [][]string) {
	newHeader := make([]string, len(idxs))
	for k, j := range idxs {
		newHeader[k] = header[j]
	}
	newData := make([][]string, len(data))
	for i := range data {
		row := make([]string, len(idxs))
		for k, j := range idxs {
			if j < len(data[i]) {
				row[k] = data[i][j]
			}
		}
		newData[i] = row
	}
	return newHeader, newData
}
