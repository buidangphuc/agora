package config

import sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
