package laravel_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/init-pkg/nova-template/internal/config"
	"github.com/init-pkg/nova/errs"
)

type LaravelClient struct {
	url    string
	client *http.Client
}

// Response структуры для API
type SuccessRequest struct {
	JobID      uint64            `json:"job_id"`
	Notes      string            `json:"notes,omitempty"`
	ResultData map[string]string `json:"result_data,omitempty"`
}

type ErrorRequest struct {
	JobID        uint64 `json:"job_id"`
	ErrorMessage string `json:"error_message"`
}

type UpdateStatusRequest struct {
	JobID  uint64 `json:"job_id"`
	Status string `json:"status"`
}

type APIResponse[T any] struct {
	Message string `json:"message"`
	Data    T      `json:"data,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// Mapping structures для создания
type ProductMapping struct {
	ExcelHeader     string   `json:"excel_header"`
	ProductField    string   `json:"product_field"`
	ConfidenceScore *float64 `json:"confidence_score,omitempty"`
}

type CategoryMapping struct {
	ExcelHeader     string   `json:"excel_header"`
	CategoryID      uint64   `json:"category_id"`
	ConfidenceScore *float64 `json:"confidence_score,omitempty"`
}

type BrandMapping struct {
	ExcelHeader     string   `json:"excel_header"`
	BrandID         uint64   `json:"brand_id"`
	ConfidenceScore *float64 `json:"confidence_score,omitempty"`
}

type CreateProductMappingsRequest struct {
	Mappings   []ProductMapping `json:"mappings"`
	SupplierID *uint64          `json:"supplier_id,omitempty"`
}

type CreateCategoryMappingsRequest struct {
	Mappings   []CategoryMapping `json:"mappings"`
	SupplierID *uint64           `json:"supplier_id,omitempty"`
}

type CreateBrandMappingsRequest struct {
	Mappings   []BrandMapping `json:"mappings"`
	SupplierID *uint64        `json:"supplier_id,omitempty"`
}

type ProductMappingResponse struct {
	ID           uint64  `json:"id"`
	ExcelHeader  string  `json:"excel_header"`
	ProductField string  `json:"product_field"`
	SupplierID   *uint64 `json:"supplier_id"`
	SupplierName *string `json:"supplier_name"`
}

type CategoryMappingResponse struct {
	ID           uint64  `json:"id"`
	ExcelHeader  string  `json:"excel_header"`
	CategoryID   uint64  `json:"category_id"`
	CategoryName *string `json:"category_name"`
	SupplierID   *uint64 `json:"supplier_id"`
	SupplierName *string `json:"supplier_name"`
}

type BrandMappingResponse struct {
	ID           uint64  `json:"id"`
	ExcelHeader  string  `json:"excel_header"`
	BrandID      uint64  `json:"brand_id"`
	BrandName    *string `json:"brand_name"`
	SupplierID   *uint64 `json:"supplier_id"`
	SupplierName *string `json:"supplier_name"`
}

// QueryParams структура для query параметров
type QueryParams struct {
	SupplierName *string `json:"supplier_name,omitempty"`
	SupplierID   *uint64 `json:"supplier_id,omitempty"`
	OnlyGlobal   bool    `json:"only_global,omitempty"`
}

func New(cfg *config.Config) *LaravelClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &LaravelClient{
		url:    cfg.Clients.Laravel.Url,
		client: client,
	}
}

// MarkJobSuccess - отмечает задачу как успешно выполненную
func (this *LaravelClient) MarkJobSuccess(jobID uint64, notes string, resultData map[string]string) errs.Error {
	url := fmt.Sprintf("%s/api/excel-jobs/mark-success", this.url)

	payload := SuccessRequest{
		JobID:      jobID,
		Notes:      notes,
		ResultData: resultData,
	}

	jsonData, e := json.Marshal(payload)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req, e := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Content-Type", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	return nil
}

// MarkJobFailed - отмечает задачу как неудачную
func (this *LaravelClient) MarkJobFailed(jobID uint64, errorMessage string) errs.Error {
	url := fmt.Sprintf("%s/api/excel-jobs/mark-error", this.url)

	payload := ErrorRequest{
		JobID:        jobID,
		ErrorMessage: errorMessage,
	}

	js, e := json.Marshal(payload)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req, e := http.NewRequest("POST", url, bytes.NewBuffer(js))
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Content-Type", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	return nil
}

// UpdateJobStatus - обновляет статус задачи в очереди
func (this *LaravelClient) UpdateJobStatus(jobID uint64, status string) errs.Error {
	url := fmt.Sprintf("%s/api/excel-jobs/update-status", this.url)

	payload := UpdateStatusRequest{
		JobID:  jobID,
		Status: status,
	}

	jsonData, e := json.Marshal(payload)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req, e := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Content-Type", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	return nil
}

// GetProductMappings - получает маппинги товаров с query параметрами
func (this *LaravelClient) GetProductMappings(params *QueryParams) ([]ProductMappingResponse, errs.Error) {
	url := fmt.Sprintf("%s/api/excel-mappings/products", this.url)
	return this.getProductMappingsWithParams(url, params)
}

// GetCategoryMappings - получает маппинги категорий с query параметрами
func (this *LaravelClient) GetCategoryMappings(params *QueryParams) ([]CategoryMappingResponse, errs.Error) {
	url := fmt.Sprintf("%s/api/excel-mappings/categories", this.url)
	return this.getCategoryMappingsWithParams(url, params)
}

// GetBrandMappings - получает маппинги брендов с query параметрами
func (this *LaravelClient) GetBrandMappings(params *QueryParams) ([]BrandMappingResponse, errs.Error) {
	url := fmt.Sprintf("%s/api/excel-mappings/brands", this.url)
	return this.getBrandMappingsWithParams(url, params)
}

// CreateProductMappings - создает маппинги товаров
func (this *LaravelClient) CreateProductMappings(mappings []ProductMapping, supplierID *uint64) errs.Error {
	url := fmt.Sprintf("%s/api/excel-mappings/products", this.url)

	payload := CreateProductMappingsRequest{
		Mappings:   mappings,
		SupplierID: supplierID,
	}

	return this.createMappings(url, payload)
}

// CreateCategoryMappings - создает маппинги категорий
func (this *LaravelClient) CreateCategoryMappings(mappings []CategoryMapping, supplierID *uint64) errs.Error {
	url := fmt.Sprintf("%s/api/excel-mappings/categories", this.url)

	payload := CreateCategoryMappingsRequest{
		Mappings:   mappings,
		SupplierID: supplierID,
	}

	return this.createMappings(url, payload)
}

// CreateBrandMappings - создает маппинги брендов
func (this *LaravelClient) CreateBrandMappings(mappings []BrandMapping, supplierID *uint64) errs.Error {
	url := fmt.Sprintf("%s/api/excel-mappings/brands", this.url)

	payload := CreateBrandMappingsRequest{
		Mappings:   mappings,
		SupplierID: supplierID,
	}

	return this.createMappings(url, payload)
}

// Вспомогательные методы
func (this *LaravelClient) getProductMappingsWithParams(baseURL string, params *QueryParams) ([]ProductMappingResponse, errs.Error) {
	parsedURL, e := url.Parse(baseURL)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	query := parsedURL.Query()

	if params != nil {
		if params.SupplierName != nil {
			query.Set("supplier_name", *params.SupplierName)
		}
		if params.SupplierID != nil {
			query.Set("supplier_id", fmt.Sprintf("%d", *params.SupplierID))
		}
		if params.OnlyGlobal {
			query.Set("only_global", "true")
		}
	}

	parsedURL.RawQuery = query.Encode()

	req, e := http.NewRequest("GET", parsedURL.String(), nil)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Accept", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return nil, errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	var apiResp APIResponse[[]ProductMappingResponse]
	if e := json.NewDecoder(res.Body).Decode(&apiResp); e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	return apiResp.Data, nil
}

func (this *LaravelClient) getCategoryMappingsWithParams(baseURL string, params *QueryParams) ([]CategoryMappingResponse, errs.Error) {
	parsedURL, e := url.Parse(baseURL)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	query := parsedURL.Query()

	if params != nil {
		if params.SupplierName != nil {
			query.Set("supplier_name", *params.SupplierName)
		}
		if params.SupplierID != nil {
			query.Set("supplier_id", fmt.Sprintf("%d", *params.SupplierID))
		}
		if params.OnlyGlobal {
			query.Set("only_global", "true")
		}
	}

	parsedURL.RawQuery = query.Encode()

	req, e := http.NewRequest("GET", parsedURL.String(), nil)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Accept", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return nil, errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	var apiResp APIResponse[[]CategoryMappingResponse]
	if e := json.NewDecoder(res.Body).Decode(&apiResp); e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	return apiResp.Data, nil
}

func (this *LaravelClient) getBrandMappingsWithParams(baseURL string, params *QueryParams) ([]BrandMappingResponse, errs.Error) {
	parsedURL, e := url.Parse(baseURL)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	query := parsedURL.Query()

	if params != nil {
		if params.SupplierName != nil {
			query.Set("supplier_name", *params.SupplierName)
		}
		if params.SupplierID != nil {
			query.Set("supplier_id", fmt.Sprintf("%d", *params.SupplierID))
		}
		if params.OnlyGlobal {
			query.Set("only_global", "true")
		}
	}

	parsedURL.RawQuery = query.Encode()

	req, e := http.NewRequest("GET", parsedURL.String(), nil)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Accept", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return nil, errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	var apiResp APIResponse[[]BrandMappingResponse]
	if e := json.NewDecoder(res.Body).Decode(&apiResp); e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	return apiResp.Data, nil
}

func (this *LaravelClient) createMappings(url string, payload interface{}) errs.Error {
	jsonData, e := json.Marshal(payload)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req, e := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	req.Header.Set("Content-Type", "application/json")

	res, e := this.client.Do(req)
	if e != nil {
		return errs.WrapAppError(e, &errs.ErrorOpts{})
	}
	defer res.Body.Close()

	if res.StatusCode != 201 {
		body, _ := io.ReadAll(res.Body)
		return errs.WrapAppError(fmt.Errorf("API error %d: %s", res.StatusCode, string(body)), &errs.ErrorOpts{})
	}

	return nil
}
