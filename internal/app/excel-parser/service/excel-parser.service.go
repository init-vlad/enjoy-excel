package excel_parser_service

import (
	"encoding/json"
	"log/slog"
	"os"

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

func (this *ExcelParserService) Parse(ctx nova_ctx.Ctx, file []byte) (*app.ParseExcelResult, errs.Error) {
	res, err := this.parse(file)
	if err != nil {
		return nil, errs.WrapAppError(err, &errs.ErrorOpts{})
	}

	js, err := json.Marshal(res)
	if err != nil {
		return nil, errs.WrapAppError(err, &errs.ErrorOpts{})
	}

	os.WriteFile("parsed_result.json", js, 0644)
	return res, nil
}
