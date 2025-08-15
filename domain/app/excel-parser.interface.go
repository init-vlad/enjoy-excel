package app

import (
	"github.com/init-pkg/nova/errs"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"
)

type ParseExcelResult struct {
	Header []string   `json:"header"`
	Rows   [][]string `json:"rows"`
}

type ExcelParserService interface {
	Parse(ctx nova_ctx.Ctx, file []byte) (*ParseExcelResult, errs.Error)
}
