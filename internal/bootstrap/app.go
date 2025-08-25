package bootstrap

import (
	excel_parser_module "github.com/init-pkg/nova-template/internal/app/excel-parser"
	mapping_service "github.com/init-pkg/nova-template/internal/app/mapping/general"
	header_mapping_service "github.com/init-pkg/nova-template/internal/app/mapping/header"
	semantic_search_service "github.com/init-pkg/nova-template/internal/app/semantic-search"
	"go.uber.org/fx"
)

func appOptions() fx.Option {
	return fx.Options(
		excel_parser_module.Register(),

		fx.Provide(
			semantic_search_service.New,
			mapping_service.New,
			header_mapping_service.New,
		),

		// test
		fx.Invoke(
			TestApp,
		),
	)
}
