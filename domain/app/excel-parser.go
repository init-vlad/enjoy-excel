package app

import "github.com/init-pkg/nova/errs"

type ParseExcelResult struct {
	Header []string   `json:"header"`
	Rows   [][]string `json:"rows"`
}

type ExcelParserService interface {
	Parse() (*ParseExcelResult, errs.Error)
}
