package excel_parser_service

// import (
// 	"context"
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"encoding/json"
// 	"fmt"
// 	"regexp"
// 	"strings"
// 	"sync"

// 	"github.com/openai/openai-go/v2"
// )

// // ---------- кэш ----------

// type HeaderBounds struct {
// 	Start int // включительно, индекс строки внутри block
// 	End   int // включительно, индекс строки внутри block
// }

// type HeaderCache interface {
// 	Get(ctx context.Context, key string) (HeaderBounds, bool)
// 	Set(ctx context.Context, key string, b HeaderBounds)
// }

// type memHeaderCache struct{ m sync.Map }

// func (c *memHeaderCache) Get(ctx context.Context, key string) (HeaderBounds, bool) {
// 	v, ok := c.m.Load(key)
// 	if !ok {
// 		return HeaderBounds{}, false
// 	}
// 	return v.(HeaderBounds), true
// }
// func (c *memHeaderCache) Set(ctx context.Context, key string, b HeaderBounds) { c.m.Store(key, b) }

// // ключ кэша: хеш склеенных «компактных» строк хедера (которые вернул LLM)
// func cacheKeyFromHeader(block [][]string, hb HeaderBounds) (string, bool) {
// 	if hb.Start < 0 || hb.End < hb.Start || hb.End >= len(block) {
// 		return "", false
// 	}
// 	var lines []string
// 	for i := hb.Start; i <= hb.End; i++ {
// 		t := rowCompactText(block[i])
// 		if t != "" {
// 			lines = append(lines, t)
// 		}
// 	}
// 	if len(lines) == 0 {
// 		return "", false
// 	}
// 	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
// 	return hex.EncodeToString(h[:]), true
// }

// // fallback ключ — «первая строка похожая на шапку» (если LLM не отработал)
// func cacheKeyHeaderish(block [][]string) (string, bool) {
// 	for i := 0; i < min(12, len(block)); i++ {
// 		if headerKeywordHits(block[i]) >= 2 {
// 			t := rowCompactText(block[i])
// 			if t == "" {
// 				continue
// 			}
// 			h := sha256.Sum256([]byte(t))
// 			return hex.EncodeToString(h[:]), true
// 		}
// 	}
// 	// просто первая непустая
// 	for i := 0; i < min(12, len(block)); i++ {
// 		t := rowCompactText(block[i])
// 		if t != "" {
// 			h := sha256.Sum256([]byte(t))
// 			return hex.EncodeToString(h[:]), true
// 		}
// 	}
// 	return "", false
// }

// // ---------- LLM: ищем границы хедера ----------

// // мы даём модели первые N строк блока в виде «index: text» и просим вернуть JSON
// // {"header_start": int, "header_end": int} (0-базовая индексация), где [start..end] — contiguous header.
// func (s *ExcelParserService) mlFindHeaderBounds(ctx context.Context, block [][]string) (HeaderBounds, bool) {
// 	if s.openaiClient == nil || len(block) == 0 {
// 		return HeaderBounds{}, false
// 	}

// 	N := min(24, len(block))
// 	var lines []string
// 	for i := 0; i < N; i++ {
// 		txt := rowCompactText(block[i])
// 		if txt == "" {
// 			txt = "(пусто)"
// 		}
// 		lines = append(lines, fmt.Sprintf("%d\t%s", i, txt))
// 	}

// 	sys := "Ты — помощник по структурированию табличных прайс-листов. Твоя задача — найти верхнюю шапку таблицы."
// 	user := fmt.Sprintf(`Ниже первые строки блока таблицы (индексация 0-based):

// %s

// Правила:
// - "Шапка" — это подряд идущие верхние строки с названиями столбцов, иногда в 1–3 строки.
// - Описание товара — НЕ шапка.
// - Встречаются объединённые ячейки и пустые разделители.
// - Верни ТОЛЬКО JSON без пояснений строго в формате:
// {"header_start": <int>, "header_end": <int>}

// Требования:
// - Индексы относительны к списку строк выше.
// - header_start ≤ header_end.
// - Если сомневаешься, выбери минимально достаточную шапку (обычно 1–2 строки).`, strings.Join(lines, "\n"))

// 	// Используем простую chat-completions и парсим JSON из текста (без JSON-mode, чтоб не зависеть от версии SDK).
// 	resp, err := s.openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
// 		Model: openai.ChatModelGPT5Nano,
// 		Messages: []openai.ChatCompletionMessageParamUnion{
// 			openai.SystemMessage(sys),
// 			openai.UserMessage(user),
// 		},
// 		Temperature: openai.Float(0.0),
// 	})
// 	if err != nil || len(resp.Choices) == 0 {
// 		return HeaderBounds{}, false
// 	}

// 	// извлечём текст; у v2 SDK Content у сообщения — строка
// 	text := ""
// 	if len(resp.Choices) > 0 {
// 		text = resp.Choices[0].Message.Content
// 	}

// 	// выдёргиваем JSON двумя способами (на случай префикса/суффикса)
// 	// (а) строгий JSON
// 	var tmp struct {
// 		Start int `json:"header_start"`
// 		End   int `json:"header_end"`
// 	}
// 	if json.Unmarshal([]byte(text), &tmp) == nil {
// 		if tmp.Start < 0 || tmp.End < tmp.Start || tmp.End >= N {
// 			return HeaderBounds{}, false
// 		}
// 		return HeaderBounds{Start: tmp.Start, End: tmp.End}, true
// 	}

// 	// (б) регэкспом числа
// 	re := regexp.MustCompile(`"header_start"\s*:\s*(\d+)[^0-9]+"header_end"\s*:\s*(\d+)`)
// 	m := re.FindStringSubmatch(text)
// 	if len(m) == 3 {
// 		var st, en int
// 		_, _ = fmt.Sscanf(m[1], "%d", &st)
// 		_, _ = fmt.Sscanf(m[2], "%d", &en)
// 		if st >= 0 && en >= st && en < N {
// 			return HeaderBounds{Start: st, End: en}, true
// 		}
// 	}

// 	return HeaderBounds{}, false
// }
