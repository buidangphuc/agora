// Package config holds team-referral's settings: the SDK groups it uses, bound
// from environment variables.
package config

import sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

// Settings is everything the service reads from the environment. Runtime and
// GRPC are embedded because nothing else collides with their fields; Postgres
// and OTel are named because both have an Enabled field.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
}

// Validate runs after parsing; sdkconfig.Load calls it.
func (s Settings) Validate() error { return s.Postgres.Validate() }

// Load reads Settings from the environment and validates it.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
