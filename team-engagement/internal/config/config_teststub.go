package config

import "errors"

type MigrateSettings struct{}

func Load() (Settings, error) { return Settings{}, errors.New("TEST-STUB: Load not implemented") }

func LoadMigrate() (MigrateSettings, error) {
	return MigrateSettings{}, errors.New("TEST-STUB: LoadMigrate not implemented")
}
