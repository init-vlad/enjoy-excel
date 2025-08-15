package excel_parser_service

// import (
// 	"context"
// 	"encoding/json"
// 	"log/slog"
// 	"os"

// 	"github.com/init-pkg/nova-template/domain/app"
// 	"github.com/init-pkg/nova/errs"
// 	nova_ctx "github.com/init-pkg/nova/shared/ctx"

// 	"github.com/openai/openai-go/v2"
// )

// // ---------------------------------------------------------------
// // Service
// // ---------------------------------------------------------------

// type ExcelParserService struct {
// 	openaiClient *openai.Client
// 	log          *slog.Logger
// 	cache        HeaderCache
// }

// var _ app.ExcelParserService = &ExcelParserService{}

// func New(openaiClient *openai.Client, log *slog.Logger) *ExcelParserService {
// 	return &ExcelParserService{openaiClient, log, &memHeaderCache{}}
// }

// // Parse теперь возвращает массив результатов по всем найденным таблицам.
// func (s *ExcelParserService) Parse(ctx nova_ctx.Ctx, file []byte) ([]*app.ParseExcelResult, errs.Error) {
// 	results, err := s.parse(context.Background(), file)
// 	if err != nil {
// 		return nil, errs.WrapAppError(err, &errs.ErrorOpts{})
// 	}

// 	js, err2 := json.MarshalIndent(results, "", "  ")
// 	if err2 == nil {
// 		_ = os.WriteFile("parsed_result.json", js, 0644)
// 	}

// 	return results, nil
// }
