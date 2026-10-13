// Package config holds team-order's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Flags, Kafka, Outbox) plus the groups below,
// bound from flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load.
// The `env`/`default` struct tags are the single source of truth for both
// loading and the .env.example drift test (config_test.go).
//
// There are no enable flags for always-on capabilities (database, Kafka, the
// outbox relayer, reflection, feature flags). OTEL_ENABLED stays: it is a real
// toggle.
package config

import (
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide (Enabled).
// sdkconfig.Load validates every group, so Settings has no Validate of its own.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres    sdkconfig.Postgres
	OTel        sdkconfig.OTel
	Flags       sdkconfig.Flags
	Kafka       sdkconfig.Kafka
	Outbox      sdkconfig.Outbox
	Events      Events
	Upstream    Upstream
	Reservation Reservation
}

// Upstream holds the addresses of the services team-order calls.
type Upstream struct {
	DomainAddr    string `env:"UPSTREAM_DOMAIN_ADDR" default:"localhost:50051"`
	IdentityAddr  string `env:"UPSTREAM_IDENTITY_ADDR" default:"localhost:50053"`
	PromotionAddr string `env:"UPSTREAM_PROMOTION_ADDR" default:""`
}

// Events names the Kafka topics and the consumer group. The PaymentSettled
// consumer reads PaymentTopic, parks poison records on PaymentDLQ, and the outbox
// relayer produces to OrderTopic. Both are always on.
type Events struct {
	ConsumerGroup string `env:"ORDER_PAYMENT_CONSUMER_GROUP" default:"team-order.payment"`
	PaymentTopic  string `env:"PAYMENT_EVENTS_TOPIC" default:"payment.events"`
	PaymentDLQ    string `env:"PAYMENT_EVENTS_DLQ_TOPIC" default:"payment.events.dlq"`
	OrderTopic    string `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
}

// Reservation tunes the stock-reservation lifetime and the reservation sweeper.
// Both are Go duration strings ("20s", "1m", "15m"); read them through
// ReservationTTL and ReservationSweepInterval, which never fail: an unusable value
// falls back to the default with a warning so a typo cannot stop the service. An
// explicitly empty variable is kept empty (keepempty) so it gets that warning too.
type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m" keepempty:"true"`
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m" keepempty:"true"`
}

// DefaultReservationTTL is used when RESERVATION_TTL is empty or unusable. It
// matches team-domain's own default.
const DefaultReservationTTL = 15 * time.Minute

// DefaultReservationSweepInterval is used when RESERVATION_SWEEP_INTERVAL is empty
// or unusable.
const DefaultReservationSweepInterval = time.Minute

// Load reads Settings from the environment, applies defaults and validates every
// group. It is the entry point of cmd/server and cmd/resync-commits.
func Load() (Settings, error) {
	return sdkconfig.Load[Settings]()
}

// ReservationTTL parses RESERVATION_TTL, the lifetime of a checkout's stock hold
// before the sweep may reclaim it (and of an unfinished checkout attempt). An
// empty, unparsable, zero or negative value yields the default and a non-empty
// warning naming the variable for the caller to log.
func (s *Settings) ReservationTTL() (time.Duration, string) {
	return positiveDuration("RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

// ReservationSweepInterval parses RESERVATION_SWEEP_INTERVAL, how often the
// reservation sweeper runs. Same fallback rules as ReservationTTL.
func (s *Settings) ReservationSweepInterval() (time.Duration, string) {
	return positiveDuration("RESERVATION_SWEEP_INTERVAL", s.Reservation.SweepInterval, DefaultReservationSweepInterval)
}

// positiveDuration is sdkconfig.PositiveDuration that falls back to def with a
// warning instead of failing, for a value that is empty or not a positive duration.
func positiveDuration(key, raw string, def time.Duration) (time.Duration, string) {
	d, err := sdkconfig.PositiveDuration(key, raw, def)
	if strings.TrimSpace(raw) == "" {
		err = fmt.Errorf("%s is empty: want a positive Go duration such as 20s or 15m", key)
	}
	if err != nil {
		return def, fmt.Sprintf("%v; using default %s", err, def)
	}
	return d, ""
}
