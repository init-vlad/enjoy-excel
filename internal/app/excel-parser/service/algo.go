package excel_parser_service

// import (
// 	"fmt"
// 	"math"
// 	"regexp"
// 	"strings"
// )

// func stackTop[T any](s []T) int { return len(s) - 1 }

// func cosine(a, b []float32) float64 {
// 	var dot, na, nb float64
// 	for i := range a {
// 		ai, bi := float64(a[i]), float64(b[i])
// 		dot += ai * bi
// 		na += ai * ai
// 		nb += bi * bi
// 	}
// 	if na == 0 || nb == 0 {
// 		return 0
// 	}
// 	return dot / (math.Sqrt(na) * math.Sqrt(nb))
// }

// // A1 → (row,col)
// func refToRC(ref string) (int, int) {
// 	re := regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)
// 	m := re.FindStringSubmatch(ref)
// 	if m == nil {
// 		return 0, 0
// 	}
// 	col := titleToNumber(m[1])
// 	row := 0
// 	fmt.Sscanf(m[2], "%d", &row)
// 	return row, col
// }

// func titleToNumber(s string) int {
// 	s = strings.ToUpper(s)
// 	n := 0
// 	for i := 0; i < len(s); i++ {
// 		n = n*26 + int(s[i]-'A') + 1
// 	}
// 	return n
// }
