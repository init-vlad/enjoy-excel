package amqp_module

import (
	"github.com/init-pkg/nova-template/internal/config"
	app_amqp "github.com/init-pkg/nova-template/internal/transports/amqp/client"

	"go.uber.org/fx"
)

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			func(cfg *config.Config) *app_amqp.Connection {
				res, err := app_amqp.New(&cfg.Transports.Amqp)
				if err != nil {
					panic("failed create rabbit mq: " + err.Error())
				}

				return res
			},
		),

		fx.Invoke(
			func(r *app_amqp.Connection, lc fx.Lifecycle) {
				lc.Append(fx.StopHook(r.Close))
			},
		),
	)
}
