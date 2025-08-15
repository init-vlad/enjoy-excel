package bootstrap

import (
	"os"

	"github.com/init-pkg/nova-template/domain/app"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"
)

func TestApp(parseExcelService app.ExcelParserService) {
	f, err := os.ReadFile("./resources/test-data/excel/abris-1.xlsx")
	if err != nil {
		panic(err)
	}

	parseExcelService.Parse(nova_ctx.New(), f)
}
