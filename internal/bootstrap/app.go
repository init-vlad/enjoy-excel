package bootstrap

import (
	excel_parser_module "github.com/init-pkg/nova-template/internal/app/excel-parser"
	"go.uber.org/fx"
)

func appOptions() fx.Option {
	return fx.Options(
		excel_parser_module.Register(),

		// test
		fx.Invoke(
			TestApp,
		),
	)
}
