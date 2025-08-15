package excel_parser_service

import (
	"math"
	"strings"
)

// --- helpers: плотность непустых в колонке/строке на отрезке
func nonEmptyInCol(rows [][]string, c, r1, r2 int) (cnt, total int) {
	for r := r1; r <= r2 && r < len(rows); r++ {
		if c < len(rows[r]) && strings.TrimSpace(rows[r][c]) != "" {
			cnt++
		}
		total++
	}
	return
}
func nonEmptyInRow(rows [][]string, r, c1, c2 int) (cnt, total int) {
	if r >= len(rows) {
		return 0, 0
	}
	for c := c1; c <= c2; c++ {
		if c < len(rows[r]) && strings.TrimSpace(rows[r][c]) != "" {
			cnt++
		}
		total++
	}
	return
}

// --- горизонтальное расширение от уже найденного блока
// смотрим окно первых W строк данных и добавляем колонки, где непустых >= minHits
func expandRectHoriz(rows [][]string, rect Rect, headerDepth int, maxCols int) Rect {
	if rect.Empty() {
		return rect
	}
	startDataRow := rect.R1 + headerDepth
	if startDataRow > rect.R2 {
		startDataRow = rect.R1 + 1
	}
	// окно для принятия решения
	W := 30
	if startDataRow+W-1 > rect.R2 {
		W = max(10, rect.R2-startDataRow+1)
	}
	r1 := startDataRow
	r2 := min(startDataRow+W-1, rect.R2)

	// вправо
	for c := rect.C2 + 1; c < maxCols; c++ {
		cnt, total := nonEmptyInCol(rows, c, r1, r2)
		minHits := 3
		if total >= 10 {
			minHits = int(math.Max(3, math.Round(float64(total)*0.25))) // ≥25% непустых
		}
		if cnt >= minHits {
			rect.C2 = c
		} else {
			// допускаем 1–2 «провала», чтобы не порваться из-за одной пустой колонки
			fail := 0
			for look := c; look < min(c+2, maxCols); look++ {
				cnt2, total2 := nonEmptyInCol(rows, look, r1, r2)
				minHits2 := 3
				if total2 >= 10 {
					minHits2 = int(math.Max(3, math.Round(float64(total2)*0.25)))
				}
				if cnt2 >= minHits2 {
					rect.C2 = look
					c = look
					fail = -1
					break
				}
				fail++
			}
			if fail >= 0 {
				break
			}
		}
	}

	// влево
	for c := rect.C1 - 1; c >= 0; c-- {
		cnt, total := nonEmptyInCol(rows, c, r1, r2)
		minHits := 3
		if total >= 10 {
			minHits = int(math.Max(3, math.Round(float64(total)*0.25)))
		}
		if cnt >= minHits {
			rect.C1 = c
		} else {
			fail := 0
			for look := c; look >= max(c-2, 0); look-- {
				cnt2, total2 := nonEmptyInCol(rows, look, r1, r2)
				minHits2 := 3
				if total2 >= 10 {
					minHits2 = int(math.Max(3, math.Round(float64(total2)*0.25)))
				}
				if cnt2 >= minHits2 {
					rect.C1 = look
					c = look
					fail = -1
					break
				}
				fail++
			}
			if fail >= 0 {
				break
			}
		}
	}

	return rect
}

// --- вертикальное расширение вниз: тянем до тех пор, пока есть содержательные строки
func extendRectDown(rows [][]string, rect Rect) Rect {
	if rect.Empty() {
		return rect
	}
	emptyRun := 0
	r := rect.R2 + 1
	for r < len(rows) {
		cnt, _ := nonEmptyInRow(rows, r, rect.C1, rect.C2)
		if cnt == 0 {
			emptyRun++
			if emptyRun >= 2 { // две пустых подряд — конец
				break
			}
		} else {
			emptyRun = 0
			rect.R2 = r
		}
		r++
	}
	return rect
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
