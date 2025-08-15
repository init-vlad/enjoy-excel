package bootstrap

import (
	"go.uber.org/fx"
)

func Run() {
	app := fx.New(
		coreOptions(),
		appOptions(),
		clientsOptions(),
	)

	app.Run()
}
