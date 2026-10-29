package config

import "errors"

// TEST-STUB: compile-only; the implementer of task 1.2 deletes this file.

type MigrateSettings struct {
	Postgres struct {
		URL string `env:"DATABASE_URL"`
	}
}

func Load() (Settings, error) { return Settings{}, errors.New("TEST-STUB: Load not implemented") }

func LoadMigrate() (MigrateSettings, error) {
	return MigrateSettings{}, errors.New("TEST-STUB: LoadMigrate not implemented")
}
