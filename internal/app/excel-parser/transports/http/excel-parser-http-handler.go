package excel_parser_http_handler

import (
	"fmt"

	"github.com/init-pkg/nova-template/domain/app"
	"github.com/init-pkg/nova-template/domain/dtos"
	mapping_service "github.com/init-pkg/nova-template/internal/app/mapping/general"
	laravel_client "github.com/init-pkg/nova-template/internal/clients/laravel"
	"github.com/init-pkg/nova/errs"
	nova_fiber "github.com/init-pkg/nova/shared/fiber"

	"github.com/gofiber/fiber/v3"
)

type ExcelParserHttpHandler struct {
	service        app.ExcelParserService
	laravelClient  *laravel_client.LaravelClient
	mappingService *mapping_service.Service
}

func New(service app.ExcelParserService, laravelClient *laravel_client.LaravelClient, mappingService *mapping_service.Service) *ExcelParserHttpHandler {
	return &ExcelParserHttpHandler{service: service, laravelClient: laravelClient, mappingService: mappingService}
}

func (this *ExcelParserHttpHandler) Register(mainApp *fiber.App) {
	var app = mainApp.Group("/")

	app.Post("/manual-upload", this.manualUpload)
}

func (this *ExcelParserHttpHandler) manualUpload(fctx fiber.Ctx) error {
	var ctx = nova_fiber.ToNovaCtx(fctx)

	req, err := nova_fiber.ParseAndValidateBodyT[dtos.ExcelParserManualUploadRequest](fctx, ctx)
	if err != nil {
		return errs.WriteError(fctx, err)
	}

	uploadFile, ok, err := nova_fiber.ExtractFileBytes(fctx, "file")
	if err != nil {
		return errs.WriteError(fctx, err)
	}

	if !ok {
		return errs.WriteError(fctx, errs.NewBadRequestError("file is required", &errs.ErrorOpts{Ctx: ctx}))
	}

	res, err := this.service.Parse(ctx, uploadFile)
	if err != nil {
		return errs.WriteError(fctx, err)
	}

	supMappings, e := this.laravelClient.GetProductMappings(&laravel_client.QueryParams{SupplierID: &req.SupplierId})
	if e != nil {
		return errs.WriteError(fctx, e)
	}
	gMappings, e := this.laravelClient.GetProductMappings(&laravel_client.QueryParams{OnlyGlobal: true})
	if e != nil {
		return errs.WriteError(fctx, e)
	}

	fmt.Println("Supplier name: ", req.SupplierName)
	for _, table := range res {
		fmt.Println("Table sheet: ", table.SheetName)

		this.mappingService.MapProductFields(req.SupplierId, table, supMappings, gMappings)
	}

	err = this.laravelClient.MarkJobFailed(req.JobId, "Not implemented yet")
	if err != nil {
		return errs.WriteError(fctx, err)
	}
	var _ = res
	return nil
}
