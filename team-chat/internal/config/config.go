// Package config holds team-chat's settings: the go-sdk groups it uses (Runtime,
// GRPC, Postgres, OTel, Kafka, Outbox) plus the service-specific groups below,
// bound from flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load. The
// `env`/`default` struct tags are the single source of truth for both loading and
// the .env.example drift test (config_test.go).
package config

import (
	"errors"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the rest are named because Postgres and OTel both have fields that
// would collide.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	// Outbox tunes the chat.events outbox relayer (OUTBOX_* env, the same names as
	// team-order). The relayer is always on.
	Outbox   sdkconfig.Outbox
	Events   Events
	Upstream Upstream
}

// Events names the topic the outbox relayer publishes ChatMessage events to. The
// producer is always on.
type Events struct {
	ChatTopic string `env:"KAFKA_CHAT_TOPIC" default:"chat.events"`
}

// Upstream holds the addresses of the services team-chat may call.
type Upstream struct {
	ListingAddr string `env:"UPSTREAM_LISTING_ADDR" default:"localhost:50051"` // team-domain gRPC address
}

// Validate runs after parsing; sdkconfig.Load calls it.
func (s Settings) Validate() error {
	return errors.Join(s.GRPC.Validate(), s.Postgres.Validate())
}

// Load reads Settings from the environment and validates it.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
