package grpc_server_module

import (
	"context"
	"net"

	"github.com/init-pkg/nova-template/internal/config"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func New() *grpc.Server {
	return grpc.NewServer()
}

func MustStart(lc fx.Lifecycle, cfg *config.Config, server *grpc.Server) {
	lis, err := net.Listen("tcp", cfg.Transports.Grpc.Addr)
	if err != nil {
		panic("failed create tcp server: " + err.Error())
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				var err = server.Serve(lis)
				if err != nil {
					panic("failed to serve grpc server: " + err.Error())
				}
			}()

			return nil
		},

		OnStop: func(ctx context.Context) error {
			server.Stop()
			return nil
		},
	})
}

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			New,
		),

		fx.Invoke(
			MustStart,
		),
	)
}
