package excel_parser_service

import (
	"log/slog"

	"github.com/init-pkg/nova-template/domain/app"
	"github.com/init-pkg/nova/errs"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"

	"github.com/openai/openai-go/v2"
)

type ExcelParserService struct {
	openaiClient *openai.Client
	log          *slog.Logger
}

var _ app.ExcelParserService = &ExcelParserService{}

func New(openaiClient *openai.Client, log *slog.Logger) *ExcelParserService {

	return &ExcelParserService{openaiClient, log}
}

func (s *ExcelParserService) Parse(ctx nova_ctx.Ctx, file []byte) (*app.ParseExcelResult, errs.Error) {
	return nil, nil
	// Implement the logic to parse Excel files here
	// This is a placeholder implementatio
}
