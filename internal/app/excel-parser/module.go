package excel_parser_module

import (
	"github.com/gofiber/fiber/v3"
	"github.com/init-pkg/nova-template/domain/app"
	excel_parser_service "github.com/init-pkg/nova-template/internal/app/excel-parser/service"
	excel_parser_http_handler "github.com/init-pkg/nova-template/internal/app/excel-parser/transports/http"
	"go.uber.org/fx"
)

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			fx.Annotate(excel_parser_service.New, fx.As(new(app.ExcelParserService))),
			excel_parser_http_handler.New,
		),

		fx.Invoke(
			func(app *fiber.App, h *excel_parser_http_handler.ExcelParserHttpHandler) {
				h.Register(app)
			},
		),
	)
}
