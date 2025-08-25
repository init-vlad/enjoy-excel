package header_mapping_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/init-pkg/nova-template/domain/app"
	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v2"
)

// ----- DOMAIN TYPES -----

// Маппинг одного заголовка к полю товара
type ProductFieldMapping struct {
	ExcelHeader     string  `json:"excel_header" jsonschema_description:"Excel column header"`
	ProductField    string  `json:"product_field" jsonschema:"enum=name,enum=price,enum=slug,enum=description,enum=quantity,enum=brand_id,enum=sku,enum=discount,enum=category_id,enum=is_popular,enum=unknown" jsonschema_description:"Product field to map to"`
	ConfidenceScore float64 `json:"confidence_score" jsonschema:"minimum=0,maximum=1" jsonschema_description:"Mapping confidence from 0 to 1"`
}

// Основной ответ
type ProductMappingResponse struct {
	Mappings []ProductFieldMapping `json:"mappings" jsonschema_description:"Array of header to field mappings"`
}

// ----- JSON SCHEMA (Structured Outputs) -----

func GenerateSchema[T any]() interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true, // компактная схема без $ref
	}
	var v T
	return reflector.Reflect(v)
}

var ProductMappingResponseSchema = GenerateSchema[ProductMappingResponse]()

// OpenAI v2: параметр схемы для response_format
var schemaParam = openai.ResponseFormatJSONSchemaJSONSchemaParam{
	Name:        "product_mapping",
	Description: openai.String("Excel headers to product fields mapping"),
	Schema:      ProductMappingResponseSchema,
	Strict:      openai.Bool(true),
}

// ----- REQUEST SHAPE ДЛЯ ВХОДА В ПОДСКАЗКУ -----

// То, что пошлём модели как компактный INPUT_JSON,
// чтобы не плодить «болтовню» в промпте.
type mappingInput struct {
	Headers       []string            `json:"headers"`
	ExampleValues map[string][]string `json:"example_values,omitempty"` // header -> sample values
}

// ----- SERVICE -----

type HeaderMappingService struct {
	openaiClient *openai.Client
	// Можно тюнить при инициализации при желании:
	maxExamplesPerHeader int
	exampleTruncateLen   int
	ctxTimeout           time.Duration
	batchSize            int // если заголовков очень много — режем на батчи
}

func New(openaiClient *openai.Client) *HeaderMappingService {
	return &HeaderMappingService{
		openaiClient:         openaiClient,
		maxExamplesPerHeader: 2,
		exampleTruncateLen:   140,
		ctxTimeout:           25 * time.Second,
		batchSize:            120, // безопасный размер контекста
	}
}

// Обратная совместимость: поведение как раньше (без исключений).
func (s *HeaderMappingService) BuildProductFieldsMapping(
	r *app.ParseExcelResult,
) (ProductMappingResponse, error) {
	return s.BuildProductFieldsMappingExcept(r, nil)
}

func normalizeHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Новый метод: скипаем уже известные заголовки (регистронезависимо).
func (s *HeaderMappingService) BuildProductFieldsMappingExcept(
	r *app.ParseExcelResult,
	skipHeaders []string,
) (ProductMappingResponse, error) {
	if r == nil || len(r.Header) == 0 {
		return ProductMappingResponse{}, errors.New("empty excel parse result")
	}

	// Множество исключений (нормализованных)
	skip := make(map[string]struct{}, len(skipHeaders))
	for _, h := range skipHeaders {
		if k := normalizeHeader(h); k != "" {
			skip[k] = struct{}{}
		}
	}

	// Список глобальных индексов колонок, которые надо маппить
	candidates := make([]int, 0, len(r.Header))
	for idx, h := range r.Header {
		if _, ok := skip[normalizeHeader(h)]; ok {
			continue
		}
		candidates = append(candidates, idx)
	}
	if len(candidates) == 0 {
		return ProductMappingResponse{Mappings: nil}, nil
	}

	allMappings := make([]ProductFieldMapping, 0, len(candidates))

	// Батчим по ГЛОБАЛЬНЫМ индексам (исправляет проблему со срезами)
	for start := 0; start < len(candidates); start += s.batchSize {
		end := start + s.batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batchIdx := candidates[start:end]

		inputJSON, err := s.buildInputJSONByIndices(r.Header, r.Rows, batchIdx)
		if err != nil {
			return ProductMappingResponse{}, fmt.Errorf("build input json: %w", err)
		}

		part, err := s.callModel(inputJSON)
		if err != nil {
			return ProductMappingResponse{}, err
		}
		allMappings = append(allMappings, part.Mappings...)
	}

	return ProductMappingResponse{Mappings: allMappings}, nil
}

// Формирует INPUT_JSON только для выбранных колонок (по их глобальным индексам).
func (s *HeaderMappingService) buildInputJSONByIndices(
	headers []string,
	rows [][]string,
	indices []int,
) (string, error) {
	seen := make(map[string]struct{}, len(indices))
	uniqIdx := make([]int, 0, len(indices))
	uniqHeaders := make([]string, 0, len(indices))

	// Дедуп по «видимому» тексту заголовка (берём первый встретившийся столбец)
	for _, idx := range indices {
		if idx < 0 || idx >= len(headers) {
			continue
		}
		h := headers[idx]
		key := normalizeHeader(h)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniqIdx = append(uniqIdx, idx)
		uniqHeaders = append(uniqHeaders, h)
	}

	examples := make(map[string][]string, len(uniqIdx))
	for i := range uniqIdx {
		col := uniqIdx[i]
		header := uniqHeaders[i]

		samples := make([]string, 0, s.maxExamplesPerHeader)
		for _, row := range rows {
			if len(samples) >= s.maxExamplesPerHeader {
				break
			}
			if col >= len(row) {
				continue
			}
			val := strings.TrimSpace(row[col])
			if val == "" {
				continue
			}
			if len(val) > s.exampleTruncateLen {
				val = val[:s.exampleTruncateLen] + fmt.Sprintf("…(+%d)", len(val)-s.exampleTruncateLen)
			}
			dup := false
			for _, ex := range samples {
				if ex == val {
					dup = true
					break
				}
			}
			if !dup {
				samples = append(samples, val)
			}
		}
		if len(samples) > 0 {
			examples[header] = samples
		}
	}

	payload := mappingInput{
		Headers:       uniqHeaders,
		ExampleValues: examples,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Один вызов модели на батч.
func (s *HeaderMappingService) callModel(inputJSON string) (ProductMappingResponse, error) {
	// Сверхкраткая роль + правила. Не «перегибаем» с текстом.
	system := "You map Excel headers to product fields from a fixed enum. Use examples to disambiguate. If unsure, use \"unknown\". Return ONLY the JSON required by the schema."

	// Пользовательское сообщение содержит только инструкцию и INPUT_JSON
	user := fmt.Sprintf("Map headers using the examples.\nINPUT_JSON:\n%s", inputJSON)

	// Контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), s.ctxTimeout)
	defer cancel()

	chat, err := s.openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
		// Строгое соответствие нашей JSON Schema (Structured Outputs)
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
		// Семя — больше повторяемости (детерминизм не гарантируется, но помогает)
		Seed:  openai.Int(42), // см. README SDK
		Model: openai.ChatModelGPT5Nano,
		// temperature можно опустить; если критично — поставить очень маленькое значение.
		// В некоторых клиентах "0" может быть опущено сериализатором, поэтому минимум > 0:
		// Temperature: openai.Float64(math.SmallestNonzeroFloat64),
	})
	if err != nil {
		return ProductMappingResponse{}, fmt.Errorf("openai chat completion: %w", err)
	}

	if len(chat.Choices) == 0 {
		return ProductMappingResponse{}, errors.New("openai: empty choices")
	}

	var mappingResponse ProductMappingResponse
	if uerr := json.Unmarshal([]byte(chat.Choices[0].Message.Content), &mappingResponse); uerr != nil {
		// На всякий случай: когда strict=false/сбой, модель могла вернуть текст.
		return ProductMappingResponse{}, fmt.Errorf("unmarshal model output: %w", uerr)
	}

	// Нормализация: гарантия допустимых значений на случай будущих расширений модели
	for i := range mappingResponse.Mappings {
		if mappingResponse.Mappings[i].ProductField == "" {
			mappingResponse.Mappings[i].ProductField = "unknown"
		}
		// Сжимаем возможные float артефакты (например, 1.0000000002)
		if mappingResponse.Mappings[i].ConfidenceScore < 0 {
			mappingResponse.Mappings[i].ConfidenceScore = 0
		} else if mappingResponse.Mappings[i].ConfidenceScore > 1 {
			mappingResponse.Mappings[i].ConfidenceScore = 1
		} else {
			mappingResponse.Mappings[i].ConfidenceScore = round2(mappingResponse.Mappings[i].ConfidenceScore)
		}
	}

	return mappingResponse, nil
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
