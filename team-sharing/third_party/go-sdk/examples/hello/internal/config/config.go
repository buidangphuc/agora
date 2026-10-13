// Package config holds the hello service's settings: the SDK groups it uses,
// bound from environment variables.
package config

import sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

// Settings is everything the service reads from the environment. Runtime and
// GRPC are embedded so their fields read as s.Env and s.Port; Postgres and OTel
// stay named groups because s.Postgres.URL and s.OTel.Enabled say what they
// configure. Add service-specific fields next to them. A service that also uses
// sdkconfig.Kafka or sdkconfig.Flags adds them as named groups: Load then
// requires KAFKA_BROKERS (and FLIPT_ADDR with flags on) in a staging or prod ENV
// and defaults them to localhost otherwise.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
}

// Load reads Settings from the environment and validates it. It calls Validate
// on every group above (Runtime, GRPC, Postgres), so Settings needs no Validate
// of its own; add one only for cross-field checks.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
