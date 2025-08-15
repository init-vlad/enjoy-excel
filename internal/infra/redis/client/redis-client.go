package app_redis

import (
	nova_configs "github.com/init-pkg/nova/lib/nova-configs"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func NewClient(cfg nova_configs.Redis) *Client {
	return &Client{
		Client: redis.NewClient(cfg.Options()),
	}
}
