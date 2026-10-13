// Package config holds team-promotion's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Kafka, Flags) plus the service-specific groups
// below, bound from flat env keys with sdkconfig.Load. The `env`/`default` tags
// are the single source of truth for both loading and the .env.example drift
// test (config_test.go).
package config

import (
	"fmt"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// maxPort is the highest TCP port GRPC_PORT may name.
const maxPort = 65535

// Settings is the whole configuration surface. The SDK groups are named fields
// so call sites read s.Server.Port, s.Database.URL.
type Settings struct {
	Runtime  sdkconfig.Runtime
	Server   sdkconfig.GRPC
	Database sdkconfig.Postgres
	OTel     sdkconfig.OTel
	// Flags keeps its Enabled switch: gitops runs promotion with
	// FEATURE_FLAGS_ENABLED=false because no Flipt is deployed there.
	Flags    sdkconfig.Flags
	Kafka    sdkconfig.Kafka
	Events   Events
	Upstream Upstream
	Ads      Ads
}

// Events names the topic promotion state-change events are published to
// (ADR-0002). Publishing is always on.
type Events struct {
	Topic string `env:"PROMOTION_EVENTS_TOPIC" default:"promotion.events"`
}

// Upstream locates team-domain, used to verify listing ownership when a seller
// creates a flash-sale campaign. Empty = ownership cannot be verified, so
// non-admin CreateCampaign fails closed (Unavailable).
type Upstream struct {
	DomainAddr string `env:"UPSTREAM_DOMAIN_ADDR" default:""`
	// CallTimeoutSeconds bounds one team-domain lookup, so a dead upstream yields
	// UNAVAILABLE instead of consuming the whole request budget (a gateway 504).
	CallTimeoutSeconds float64 `env:"UPSTREAM_CALL_TIMEOUT_SECONDS" default:"2"`
}

// Ads bounds what a seller may put on a sponsored campaign (minor units). Both
// must be > 0.
type Ads struct {
	MaxBid    int64 `env:"MAX_AD_BID" default:"1000000"`
	MaxBudget int64 `env:"MAX_AD_BUDGET" default:"1000000000"`
}

// Load reads Settings from the environment and validates it (sdkconfig.Load
// calls Validate after parsing).
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

// DeclaredEnvKeys lists every env key Settings declares, for the drift test.
func DeclaredEnvKeys() []string { return sdkconfig.DeclaredEnvKeys(Settings{}) }

// Validate enforces the invariants the service adds; sdkconfig.Load validates
// the SDK groups itself.
func (s Settings) Validate() error {
	if s.Server.Port > maxPort {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.Ads.MaxBid <= 0 {
		return fmt.Errorf("MAX_AD_BID must be > 0: %d", s.Ads.MaxBid)
	}
	if s.Ads.MaxBudget <= 0 {
		return fmt.Errorf("MAX_AD_BUDGET must be > 0: %d", s.Ads.MaxBudget)
	}
	if s.Upstream.CallTimeoutSeconds <= 0 {
		return fmt.Errorf("UPSTREAM_CALL_TIMEOUT_SECONDS must be > 0: %v", s.Upstream.CallTimeoutSeconds)
	}
	return nil
}
