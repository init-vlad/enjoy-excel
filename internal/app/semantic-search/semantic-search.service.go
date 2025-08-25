package semantic_search_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// SearchResult представляет результат семантического поиска
type SearchResult struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// Service предоставляет семантический поиск категорий и брендов
type Service struct {
	openaiClient     *openai.Client
	opensearchClient *opensearchapi.Client
	categoryIndex    string
	brandIndex       string
	embeddingModel   string
}

// NewService создает новый экземпляр Service
func New(
	openaiClient *openai.Client,
	opensearchClient *opensearchapi.Client,
) *Service {
	return &Service{
		openaiClient:     openaiClient,
		opensearchClient: opensearchClient,
		categoryIndex:    "categories",
		brandIndex:       "brands",
		embeddingModel:   openai.EmbeddingModelTextEmbedding3Small,
	}
}

// generateEmbedding генерирует эмбеддинг для текста
func (this *Service) generateEmbedding(ctx context.Context, text string) ([]float64, error) {
	response, err := this.openaiClient.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: []string{text},
		},
		Model: this.embeddingModel,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(response.Data) == 0 {
		return nil, errors.New("no embedding data received")
	}

	return response.Data[0].Embedding, nil
}

// searchInIndex выполняет kNN поиск в указанном индексе
func (s *Service) searchInIndex(ctx context.Context, index string, embedding []float64, k int, minScore float64) ([]SearchResult, error) {
	// Создаем запрос для kNN поиска
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"knn": map[string]interface{}{
				"embedding": map[string]interface{}{
					"vector": embedding,
					"k":      k,
				},
			},
		},
		"size":      k,
		"_source":   []string{"id", "name"},
		"min_score": minScore,
	}

	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Выполняем поиск
	searchResp, err := s.opensearchClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{index},
		Body:    strings.NewReader(string(queryJSON)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search in index %s: %w", index, err)
	}

	// Парсим результаты
	var results []SearchResult
	for _, hit := range searchResp.Hits.Hits {
		var source struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}

		if err := json.Unmarshal(hit.Source, &source); err != nil {
			continue
		}

		results = append(results, SearchResult{
			ID:         source.ID,
			Name:       source.Name,
			Confidence: float64(hit.Score),
		})
	}

	return results, nil
}

// FindBestCategory ищет наиболее подходящую категорию по названию
func (s *Service) FindBestCategory(ctx context.Context, name string) (*SearchResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("category name cannot be empty")
	}

	// Генерируем эмбеддинг
	embedding, err := s.generateEmbedding(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for category '%s': %w", name, err)
	}

	// Ищем в индексе категорий
	results, err := s.searchInIndex(ctx, s.categoryIndex, embedding, 5, 0.5)
	if err != nil {
		return nil, fmt.Errorf("failed to search categories for '%s': %w", name, err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching category found for '%s'", name)
	}

	// Возвращаем лучший результат
	return &results[0], nil
}

// FindBestBrand ищет наиболее подходящий бренд по названию
func (s *Service) FindBestBrand(ctx context.Context, name string) (*SearchResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("brand name cannot be empty")
	}

	// Генерируем эмбеддинг
	embedding, err := s.generateEmbedding(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for brand '%s': %w", name, err)
	}

	// Ищем в индексе брендов
	results, err := s.searchInIndex(ctx, s.brandIndex, embedding, 5, 0.5)
	if err != nil {
		return nil, fmt.Errorf("failed to search brands for '%s': %w", name, err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching brand found for '%s'", name)
	}

	// Возвращаем лучший результат
	return &results[0], nil
}

// FindMultipleCategories ищет несколько подходящих категорий
func (s *Service) FindMultipleCategories(ctx context.Context, name string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("category name cannot be empty")
	}

	embedding, err := s.generateEmbedding(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for category '%s': %w", name, err)
	}

	results, err := s.searchInIndex(ctx, s.categoryIndex, embedding, limit, 0.3)
	if err != nil {
		return nil, fmt.Errorf("failed to search categories for '%s': %w", name, err)
	}

	return results, nil
}

// FindMultipleBrands ищет несколько подходящих брендов
func (s *Service) FindMultipleBrands(ctx context.Context, name string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("brand name cannot be empty")
	}

	embedding, err := s.generateEmbedding(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for brand '%s': %w", name, err)
	}

	results, err := s.searchInIndex(ctx, s.brandIndex, embedding, limit, 0.3)
	if err != nil {
		return nil, fmt.Errorf("failed to search brands for '%s': %w", name, err)
	}

	return results, nil
}

// GetConfidenceLevel возвращает уровень уверенности в виде строки
func (r *SearchResult) GetConfidenceLevel() string {
	switch {
	case r.Confidence >= 0.9:
		return "very_high"
	case r.Confidence >= 0.8:
		return "high"
	case r.Confidence >= 0.7:
		return "medium"
	case r.Confidence >= 0.5:
		return "low"
	default:
		return "very_low"
	}
}

// IsAcceptable проверяет, приемлем ли результат для автоматического маппинга
// func (r *SearchResult) IsAcceptable() bool {
// 	return r.Confidence >= 0.7 // Порог для автоматического принятия
// }

// // Пример использования
// func exampleUsage() {
// 	ctx := context.Background()

// 	// Инициализация клиентов (как в предоставленных примерах)
// 	openaiClient := openai.NewClient() // с API ключом из переменной среды
// 	opensearchClient, _ := opensearchapi.NewClient(opensearchapi.Config{
// 		// конфигурация OpenSearch
// 	})

// 	// Создание сервиса
// 	searchService := NewService(openaiClient, opensearchClient)

// 	// Поиск категории
// 	categoryResult, err := searchService.FindBestCategory(ctx, "Компьютеры и ноутбуки")
// 	if err != nil {
// 		fmt.Printf("Error searching category: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("Found category: ID=%d, Name=%s, Confidence=%.3f (%s)\n",
// 		categoryResult.ID,
// 		categoryResult.Name,
// 		categoryResult.Confidence,
// 		categoryResult.GetConfidenceLevel(),
// 	)

// 	if categoryResult.IsAcceptable() {
// 		fmt.Println("Result is acceptable for automatic mapping")
// 	}

// 	// Поиск бренда
// 	brandResult, err := searchService.FindBestBrand(ctx, "Apple")
// 	if err != nil {
// 		fmt.Printf("Error searching brand: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("Found brand: ID=%d, Name=%s, Confidence=%.3f\n",
// 		brandResult.ID,
// 		brandResult.Name,
// 		brandResult.Confidence,
// 	)
// }
