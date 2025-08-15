package redis_module

import (
	"github.com/init-pkg/nova-template/internal/config"
	app_redis "github.com/init-pkg/nova-template/internal/infra/redis/client"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			func(cfg *config.Config) *app_redis.Client {
				return app_redis.NewClient(cfg.Infrastructure.Redis)
			},

			func(cl *app_redis.Client) redis.Cmdable {
				return cl
			},
		),
	)
}
