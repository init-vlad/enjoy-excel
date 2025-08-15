package main

import (
	"fmt"

	"github.com/init-pkg/nova-template/internal/config"

	nova_config_loader "github.com/init-pkg/nova/tools/config-loader"
	nova_db_initializer "github.com/init-pkg/nova/tools/db-initializer"
)

func main() {
	var (
		cfg = nova_config_loader.MustLoad[config.Config]()
		db  = nova_db_initializer.MustInit(&cfg.Infrastructure.Db)
	)

	fmt.Println("No seeders yet", db)
}
