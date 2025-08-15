package main

import (
	"github.com/init-pkg/nova-template/domain/models"
	"github.com/init-pkg/nova-template/internal/config"

	nova_config_loader "github.com/init-pkg/nova/tools/config-loader"
	nova_db_initializer "github.com/init-pkg/nova/tools/db-initializer"
)

func main() {
	var (
		cfg = nova_config_loader.MustLoad[config.Config]()
		db  = nova_db_initializer.MustInit(&cfg.Infrastructure.Db)
	)

	var err = db.Migrator().DropTable(models.Models...)
	if err != nil {
		panic("Failed to drop tables: " + err.Error())
	}

	err = db.AutoMigrate(models.Models...)
	if err != nil {
		panic("Failed to create tables: " + err.Error())
	}

}
