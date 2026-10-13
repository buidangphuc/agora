// Package config holds team-engagement's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Kafka, Outbox) plus the groups below, bound from
// flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load. The
// `env`/`default` struct tags are the single source of truth for both loading and
// the .env.example drift test (config_test.go).
//
// There are no enable flags for always-on capabilities (database, Kafka, the
// outbox relayer, reflection). OTEL_ENABLED stays: it is a real toggle.
package config

import (
	"errors"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide (Enabled).
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	Events   Events
	Upstream Upstream
}

// Events names the Kafka topics and the consumer group. The listing.events
// consumer fills the follow-feed source (seller_listings); its group starts from
// the earliest offset, so a fresh deployment backfills the feed, and records that
// keep failing park on <ListingTopic>.dlq. The outbox relayer publishes engagement
// facts to EventsTopic. Both are always on.
type Events struct {
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-engagement-feed"`
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
	EventsTopic   string `env:"ENGAGEMENT_EVENTS_TOPIC" default:"engagement.events"`
}

// Upstream holds addresses of sibling services consulted over gRPC (never their
// DBs, Rule 3). Empty OrderAddr disables verified-purchase enrichment.
type Upstream struct {
	OrderAddr string `env:"UPSTREAM_ORDER_ADDR"`
	// CallTimeoutSeconds bounds each upstream lookup (also capped to the inbound
	// request deadline minus a margin), so a dead sibling cannot hang a handler.
	CallTimeoutSeconds float64 `env:"UPSTREAM_CALL_TIMEOUT_SECONDS" default:"2"`
}

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the entry point of cmd/server.
func Load() (Settings, error) {
	return sdkconfig.Load[Settings]()
}

// Validate checks the rules of this service's own settings. The SDK groups
// (ENV, GRPC_PORT, DATABASE_URL) validate themselves: sdkconfig.Load runs every
// group's Validate, then this one, after parsing.
func (s Settings) Validate() error {
	if len(s.Kafka.BrokerList()) == 0 {
		return errors.New("KAFKA_BROKERS must list at least one broker")
	}
	if s.Events.ListingTopic == "" || s.Events.ConsumerGroup == "" {
		return errors.New("KAFKA_LISTING_TOPIC and KAFKA_CONSUMER_GROUP are required")
	}
	if s.Events.EventsTopic == "" {
		return errors.New("ENGAGEMENT_EVENTS_TOPIC is required")
	}
	if s.Outbox.PollInterval <= 0 {
		return errors.New("OUTBOX_POLL_INTERVAL must be > 0")
	}
	if s.Upstream.CallTimeoutSeconds <= 0 {
		return errors.New("UPSTREAM_CALL_TIMEOUT_SECONDS must be > 0")
	}
	return nil
}
