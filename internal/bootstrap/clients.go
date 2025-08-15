package bootstrap

import (
	openai_client "github.com/init-pkg/nova-template/internal/clients/openai"
	"go.uber.org/fx"
)

func clientsOptions() fx.Option {
	return fx.Options(
		fx.Provide(
			openai_client.New,
		),
	)
}
