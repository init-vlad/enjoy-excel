package minio_module

import (
	"github.com/init-pkg/nova-template/internal/config"
	nova_minio "github.com/init-pkg/nova/clients/minio"
	nova_minio_storage "github.com/init-pkg/nova/clients/minio/storage"
	"go.uber.org/fx"
)

func mustCreateMinioClient(cfg *config.Config) *nova_minio.Client {
	minioClient, err := nova_minio.New(&cfg.Infrastructure.Minio.Client)
	if err != nil {
		panic(err)
	}

	return minioClient
}

func mustCreateMinioStorage(minioClient *nova_minio.Client, cfg *config.Config) nova_minio_storage.Client {
	var storage = nova_minio_storage.New(minioClient, &cfg.Infrastructure.Minio.Storage)
	storage.MustInit()
	return storage
}

func Register() fx.Option {
	return fx.Options(
		fx.Provide(
			mustCreateMinioClient,
			mustCreateMinioStorage,
		),
	)
}
