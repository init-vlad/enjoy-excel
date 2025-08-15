package openai_client

import (
	"github.com/init-pkg/nova-template/internal/config"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

func New(cfg *config.Config) *openai.Client {
	var cl = openai.NewClient(
		option.WithAPIKey(cfg.Clients.OpenAI.ApiKey),
	)

	return &cl
}
