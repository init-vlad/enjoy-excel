package bootstrap

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/init-pkg/nova-template/domain/app"
	nova_ctx "github.com/init-pkg/nova/shared/ctx"
)

func TestApp(parseExcelService app.ExcelParserService) {
	var s = suite{parseExcelService}
	s.runAll()
}

type suite struct {
	parseExcelService app.ExcelParserService
}

func (this *suite) run(filename string) {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	destFolder := "./dest/test"
	os.MkdirAll(destFolder, os.ModePerm)
	destPath := filepath.Join(destFolder, name+".json")
	if _, err := os.Stat(destPath); err == nil {
		return
	}

	inputPath := filepath.Join(".", "resources", "test-data", "excel", filename)

	// Автоматическая конвертация .xls -> .xlsx
	if strings.HasSuffix(strings.ToLower(filename), ".xls") {
		xlsxName := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".xlsx"
		xlsxPath := filepath.Join(".", "resources", "test-data", "excel", xlsxName)
		// Если xlsx уже существует, не конвертируем
		if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
			cmd := exec.Command("libreoffice", "--headless", "--convert-to", "xlsx", inputPath, "--outdir", filepath.Dir(inputPath))
			if err := cmd.Run(); err != nil {
				panic("LibreOffice conversion failed: " + err.Error())
			}
		}
		inputPath = xlsxPath
	}

	f, err := os.ReadFile(inputPath)
	if err != nil {
		panic(err)
	}

	res, err := this.parseExcelService.Parse(nova_ctx.New(), f)
	if err != nil {
		panic(err)
	}

	js, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}
	os.WriteFile(destPath, js, 0644)
}

func (this *suite) runAll() {
	this.run("abris-1.xlsx")
	this.run("abris-2.xlsx")
	this.run("Comportal.xls")
	this.run("fortinet.xlsx")
	this.run("ERC.xlsx")
	this.run("FDCOM.xls")
	this.run("STN.xlsx")
}
