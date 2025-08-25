package bootstrap

import (
	laravel_client "github.com/init-pkg/nova-template/internal/clients/laravel"
	openai_client "github.com/init-pkg/nova-template/internal/clients/openai"
	opensearch_client "github.com/init-pkg/nova-template/internal/clients/opensearch"
	"go.uber.org/fx"
)

func clientsOptions() fx.Option {
	return fx.Options(
		fx.Provide(
			openai_client.New,
			opensearch_client.New,
			laravel_client.New,
		),
	)
}
