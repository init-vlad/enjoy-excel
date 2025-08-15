package main

import (
	"fmt"
	"log"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/init-pkg/nova-template/internal/config"

	nova_flag "github.com/init-pkg/nova/shared/flag"
	nova_config_loader "github.com/init-pkg/nova/tools/config-loader"
)

func main() {
	down, _ := nova_flag.Parse("down", false)
	cfg := nova_config_loader.MustLoad[config.Config]()
	dbConf := cfg.Infrastructure.Db

	driver := dbConf.Dialect
	var dsn, migrationsDir string

	switch driver {
	case "postgres":
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
			dbConf.Host, dbConf.User, dbConf.Password, dbConf.DBName, dbConf.Port, dbConf.SSLMode, dbConf.TimeZone)
		migrationsDir = filepath.Join("migrations", "postgres")
	case "mysql":
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local",
			dbConf.User, dbConf.Password, dbConf.Host, dbConf.Port, dbConf.DBName)
		migrationsDir = filepath.Join("migrations", "mysql")
	default:
		log.Fatalf("unsupported db driver: %s", driver)
	}

	db, err := goose.OpenDBWithDriver(driver, dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if down {
		if err := goose.Down(db, migrationsDir); err != nil {
			log.Fatalf("goose down failed: %v", err)
		}
		fmt.Println("Migrated down by one step.")
	} else {
		if err := goose.Up(db, migrationsDir); err != nil {
			log.Fatalf("goose up failed: %v", err)
		}
		fmt.Println("Migrations applied successfully!")
	}
}
