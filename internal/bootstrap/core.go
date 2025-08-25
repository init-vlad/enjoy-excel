package bootstrap

import (
	"log/slog"

	"github.com/init-pkg/nova-template/internal/config"
	minio_module "github.com/init-pkg/nova-template/internal/infra/minio"
	redis_module "github.com/init-pkg/nova-template/internal/infra/redis"
	http_server_module "github.com/init-pkg/nova-template/internal/transports/http"

	nova_kits_init_auth "github.com/init-pkg/nova-kits/init-auth"
	nova_logger "github.com/init-pkg/nova/lib/logger"
	nova_jwt_service "github.com/init-pkg/nova/lib/services/jwt"
	nova_config_loader "github.com/init-pkg/nova/tools/config-loader"
	nova_db_initializer "github.com/init-pkg/nova/tools/db-initializer"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func coreOptions() fx.Option {
	return fx.Options(
		/* config */
		fx.Provide(
			nova_config_loader.MustLoad[config.Config],
		),

		/* logger */
		fx.Provide(
			func(cfg *config.Config) *slog.Logger {
				return nova_logger.NewDefaultLogger(&cfg.Logger)
			},
		),

		/* infra */
		fx.Provide(
			func(cfg *config.Config) *gorm.DB {
				return nova_db_initializer.MustInit(&cfg.Infrastructure.Db)
			},
		),
		redis_module.Register(),
		minio_module.Register(),

		/* transports */
		http_server_module.Register(),
		// amqp_module.Register(),
		// grpc_server_module.Register(),

		/* auth */
		fx.Provide(
			func(log *slog.Logger, cfg *config.Config) *nova_jwt_service.Generic[nova_kits_init_auth.JwtPayload] {
				return nova_jwt_service.NewGeneric[nova_kits_init_auth.JwtPayload](log, &nova_jwt_service.Config{
					Secret: cfg.Auth.JwtSecret,
				})
			},
		),

		/* clients */
		// register your clients here
	)
}
