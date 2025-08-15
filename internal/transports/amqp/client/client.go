package app_amqp

import (
	nova_amqp "github.com/init-pkg/nova/clients/amqp"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Connection struct {
	*amqp.Connection
}

func New(cfg *nova_amqp.Config) (*Connection, error) {
	res, err := nova_amqp.New(cfg)
	if err != nil {
		return nil, err
	}

	return &Connection{res}, nil
}
