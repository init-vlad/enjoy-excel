package config

import (
	nova_amqp "github.com/init-pkg/nova/clients/amqp"
	nova_minio "github.com/init-pkg/nova/clients/minio"
	nova_minio_storage "github.com/init-pkg/nova/clients/minio/storage"
	nova_logger "github.com/init-pkg/nova/lib/logger"
	nova_configs "github.com/init-pkg/nova/lib/nova-configs"
	nova_db_initializer "github.com/init-pkg/nova/tools/db-initializer"
)

type DefaultLocale = string

type Config struct {
	App    AppConfig                `yaml:"app"`
	Logger nova_logger.LoggerConfig `yaml:"logger"`
	Auth   Auth                     `yaml:"auth"`

	Transports     Transports     `yaml:"transports"`
	Infrastructure Infrastructure `yaml:"infrastructure"`
	Clients        Clients        `yaml:"clients"`
	Internal       Internal       `yaml:"internal"`

	Monitoring Monitoring `yaml:"monitoring"` // not implemented
	Tracing    Tracing    `yaml:"tracing"`    // not implemented
}

// App metadata and environment settings
type AppConfig struct {
	DefaultLocale string `yaml:"default_locale"`
	SelfURL       string `yaml:"self_url"` // not implemented
	Env           string `yaml:"env"`      // not implemented
	Version       string `yaml:"version"`  // not implemented
}

// Transport-level configs: anything that handles communication
type Transports struct {
	Fiber nova_configs.FiberConfig `yaml:"fiber"`
	Amqp  nova_amqp.Config         `yaml:"amqp"`
	Grpc  GrpcConfig               `yaml:"grpc"`
}

type GrpcConfig struct {
	Addr string `yaml:"addr"`
}

// Infrastructure-level dependencies (DBs, caches, loggers, etc.)
type Infrastructure struct {
	Db    nova_db_initializer.Config `yaml:"db"`
	Redis nova_configs.Redis         `yaml:"redis"`
	Minio MinioConfig                `yaml:"minio"`
}

type MinioConfig struct {
	Client  nova_minio.Config         `yaml:"client"`
	Storage nova_minio_storage.Config `yaml:"storage"`
}

// Clients to external APIs, SDKs
type Clients struct {
	// Example: PaymentAPI, NotificationService, etc.
}

// Internal microservices or internal modules
type Internal struct {
	// Example: UserServiceConfig, EmailServiceConfig, etc.
}

// Security configuration
type Auth struct {
	JwtSecret string `yaml:"jwt_secret"`
}

// Observability - monitoring tools
type Monitoring struct {
	Prometheus struct {
		Enabled bool   `yaml:"enabled"` // not implemented
		Path    string `yaml:"path"`    // not implemented
	} `yaml:"prometheus"`

	Sentry struct {
		Dsn string `yaml:"dsn"` // not implemented
	} `yaml:"sentry"`
}

// Distributed tracing configuration
type Tracing struct {
	Enabled  bool   `yaml:"enabled"`  // not implemented
	Provider string `yaml:"provider"` // not implemented
	Endpoint string `yaml:"endpoint"` // not implemented
}
