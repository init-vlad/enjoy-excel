package excel_parser_http_handler

import (
	"github.com/init-pkg/nova-template/domain/app"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"

	"github.com/gofiber/fiber/v3"
)

type ExcelParserHttpHandler struct {
	service app.ExcelParserService
}

func New(service app.ExcelParserService) *ExcelParserHttpHandler {
	return &ExcelParserHttpHandler{service}
}

func (this *ExcelParserHttpHandler) Register(mainApp *fiber.App) {
	var app = mainApp.Group("/excel-parsers")

	app.Post("/manual-upload", this.manualUpload)
}

func (this *ExcelParserHttpHandler) manualUpload(fctx fiber.Ctx) error {
	var ctx = nova_ctx.Wrap(fctx.Context())
	return nil
}
