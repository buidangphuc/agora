// Package config holds team-sharing's settings: the SDK groups it uses, bound
// from environment variables.
package config

import sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

// Settings is everything the service reads from the environment. Runtime and
// GRPC are embedded so their fields read as s.Env and s.Port; Postgres and OTel
// are named groups.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
}

// Load reads Settings from the environment and validates it. Load calls
// Validate on every group above, so Settings needs no Validate of its own.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
