package excel_parser_module

import (
	"github.com/init-pkg/nova-template/domain/app"
	excel_parser_service "github.com/init-pkg/nova-template/internal/app/excel-parser/service"
	"go.uber.org/fx"
)

func Register() fx.Option {
	return fx.Provide(
		fx.Annotate(excel_parser_service.New, fx.As(new(app.ExcelParserService))),
	)
}
