// Package config holds the hello service's settings: the SDK groups it uses,
// bound from environment variables.
package config

import sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

// Settings is everything the service reads from the environment. Add
// service-specific fields next to the embedded groups.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	sdkconfig.Postgres
	sdkconfig.OTel
}

// Load reads Settings from the environment.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
