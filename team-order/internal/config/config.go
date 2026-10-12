// Package config holds team-order's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Flags, Kafka, Outbox) plus the groups below,
// bound from flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load.
// The `env`/`default` struct tags are the single source of truth for both
// loading and the .env.example drift test (config_test.go).
//
// There are no enable flags for always-on capabilities (database, Kafka, the
// outbox relayer, reflection). FEATURE_FLAGS_ENABLED and OTEL_ENABLED stay: they
// are real toggles.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide (Enabled).
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
// falls back to the default with a warning so a typo cannot stop the service.
type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m"`
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m"`
}

// DefaultReservationTTL is used when RESERVATION_TTL is empty or unusable. It
// matches team-domain's own default.
const DefaultReservationTTL = 15 * time.Minute

// DefaultReservationSweepInterval is used when RESERVATION_SWEEP_INTERVAL is empty
// or unusable.
const DefaultReservationSweepInterval = time.Minute

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the entry point of cmd/server and cmd/resync-commits.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	// The binder treats an empty value like an unset one and applies the default.
	// For the reservation durations an explicitly empty value must instead reach
	// positiveDuration, which falls back with a warning naming the variable.
	for key, field := range map[string]*string{
		"RESERVATION_TTL":            &s.Reservation.TTL,
		"RESERVATION_SWEEP_INTERVAL": &s.Reservation.SweepInterval,
	} {
		if v, ok := os.LookupEnv(key); ok && v == "" {
			*field = ""
		}
	}
	return s, nil
}

// Validate checks the SDK groups that carry their own rules: a positive
// GRPC_PORT and a non-empty DATABASE_URL. sdkconfig.Load calls it after parsing.
func (s Settings) Validate() error {
	if err := s.GRPC.Validate(); err != nil {
		return err
	}
	return s.Postgres.Validate()
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

func positiveDuration(key, raw string, def time.Duration) (time.Duration, string) {
	raw = strings.TrimSpace(raw)
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def, fmt.Sprintf("invalid %s %q (want a positive Go duration such as 20s or 15m); using default %s", key, raw, def)
	}
	return d, ""
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	return e == "prod" || e == "production"
}

// durableStorageEnvs are the ENV values (trimmed, lowercase) in which the service
// must never run on in-memory repositories. Anything else (local, test, unknown)
// is non-strict, matching the "local" default.
var durableStorageEnvs = []string{"staging", "stage", "prod", "production"}

// RequiresDurableStorage reports whether ENV names a staging/production environment.
func (s *Settings) RequiresDurableStorage() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	for _, strict := range durableStorageEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

// RequireDurableStorage is the boot guard against the silent in-memory fallback:
// in a strict ENV it fails when no database pool was obtained, so an unreachable
// database refuses to boot instead of serving orders that vanish on restart.
// Other environments always pass.
func (s *Settings) RequireDurableStorage(dbAvailable bool) error {
	if !s.RequiresDurableStorage() {
		return nil
	}
	if dbAvailable {
		return nil
	}
	return fmt.Errorf(
		"refusing to start with in-memory storage: ENV=%q requires a database (strict for ENV in %s) but the database pool is not available; set DATABASE_URL to a reachable database",
		s.Env, strings.Join(durableStorageEnvs, ", "))
}
