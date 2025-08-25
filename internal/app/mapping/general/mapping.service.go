package mapping_service

import (
	"fmt"

	"github.com/init-pkg/nova-template/domain/app"
	header_mapping_service "github.com/init-pkg/nova-template/internal/app/mapping/header"
	laravel_client "github.com/init-pkg/nova-template/internal/clients/laravel"
	"github.com/init-pkg/nova/errs"
	"github.com/openai/openai-go/v2"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

type Service struct {
	headerMappingService *header_mapping_service.HeaderMappingService
	laravelClient        *laravel_client.LaravelClient
	openaiClient         *openai.Client
	opensearchClient     *opensearchapi.Client
}

func New(headerMappingService *header_mapping_service.HeaderMappingService, laravelClient *laravel_client.LaravelClient, openaiClient *openai.Client, opensearchClient *opensearchapi.Client) *Service {
	return &Service{
		headerMappingService: headerMappingService,
		laravelClient:        laravelClient,
		openaiClient:         openaiClient,
		opensearchClient:     opensearchClient,
	}
}

/*
GO TODO:

  - маппить только новые заголовки и писать их в laravel DB. Заголовок может быть определен в unknown поле.
    Новыми считаются те, которых нет ни в одном поле из existingSupplierMappings и existingGeneralMappings.

  - в маппинге заголовков возвращать response с правильными заголовками

  - для category и brand полей тоже предзагружать и создавать маппинги

  - в парсинге excel сделать
    1. чтобы в header писалось не самое нижнее поле, а чтобы писались все значения начиная с самого верхнего в виде массива.
    т.е. теперь headers [][]string: [[цена, usd], [цена, kz]].
    2. сплитить вертикально объединенные ячейки внутри таблицы (body) c дублированием значения.

  - было бы неплохо реализовать категорию через единичное значение в row.

LARAVEL TODO:
  - реализовать unknown поле если его нет
  - реализовать прием готового json файла.
  - реализовать meta поле для товара. GO будет писать туда unknown заголовки для каждого supplier_id {supplier_id: {unknownHeader1: string}} (в конце)
*/
func (this *Service) MapProductFields(
	supplierId uint64,
	r *app.ParseExcelResult,
	existingSupplierMappings []laravel_client.ProductMappingResponse,
	existingGeneralMappings []laravel_client.ProductMappingResponse,
) (*app.ParseExcelResult, errs.Error) {

	// create skip list
	skip := make([]string, 0, len(existingSupplierMappings)+len(existingGeneralMappings))
	for _, m := range existingSupplierMappings {
		skip = append(skip, m.ExcelHeader)
	}
	for _, m := range existingGeneralMappings {
		skip = append(skip, m.ExcelHeader)
	}

	// build mapping skipping already known headers
	result, e := this.headerMappingService.BuildProductFieldsMappingExcept(r, skip)
	if e != nil {
		return nil, errs.WrapAppError(e, &errs.ErrorOpts{})
	}

	// build create laravel mappings request
	var mappingsToCreate = make([]laravel_client.ProductMapping, 0, len(result.Mappings))
	for _, m := range result.Mappings {
		if !laravel_client.ProductField(m.ProductField).IsValid() {
			continue
		}

		mappingsToCreate = append(mappingsToCreate, laravel_client.ProductMapping{
			ExcelHeader:     m.ExcelHeader,
			ProductField:    m.ProductField,
			ConfidenceScore: &m.ConfidenceScore,
		})
	}

	var err = this.laravelClient.CreateProductMappings(mappingsToCreate, &supplierId)
	fmt.Println("Created mappings: ", mappingsToCreate)
	if err != nil {
		return nil, err
	}

	// build final mapping
	var mapping = make(map[string]string, len(existingSupplierMappings)+len(existingGeneralMappings)+len(result.Mappings))
	for _, m := range existingGeneralMappings {
		mapping[m.ExcelHeader] = m.ProductField
	}

	for _, m := range existingSupplierMappings {
		mapping[m.ExcelHeader] = m.ProductField
	}

	for _, m := range result.Mappings {
		mapping[m.ExcelHeader] = m.ProductField
	}

	// build result with mapped headers
	var newR = &app.ParseExcelResult{
		Rows:      r.Rows,
		SheetName: r.SheetName,
	}

	var newHeaders = make([]string, 0, len(r.Header))
	for _, h := range r.Header {
		if v, ok := mapping[h]; ok {
			newHeaders = append(newHeaders, v)
		} else {
			fmt.Println("Header not mapped: ", h)
		}
	}

	fmt.Println("Final headers: ", newHeaders)

	newR.Header = newHeaders
	return newR, nil
}
