package http_server_module

import "go.uber.org/fx"

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			MustInit,
		),

		fx.Invoke(
			MustStart,
		),
	)
}
