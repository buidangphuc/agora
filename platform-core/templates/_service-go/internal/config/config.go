// Package config holds the service settings, bound from environment variables.
package config

import sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

// Settings is everything the service reads from the environment.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	OTel sdkconfig.OTel
}

// Load reads Settings from the environment and validates it.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
