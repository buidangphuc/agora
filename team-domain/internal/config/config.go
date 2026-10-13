// Package config holds team-domain's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Kafka, Outbox) plus the service-specific groups below, bound
// from flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load. The
// `env`/`default` struct tags are the single source of truth for both loading
// and the .env.example drift test (config_test.go).
package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded
// SDK groups; Postgres and OTel are named because both have an Enabled field.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres    sdkconfig.Postgres
	OTel        sdkconfig.OTel
	Kafka       sdkconfig.Kafka
	Outbox      sdkconfig.Outbox
	Storage     Storage
	Events      Events
	Reservation Reservation
}

// Storage configures the S3/MinIO object store for product media.
type Storage struct {
	Endpoint      string `env:"STORAGE_ENDPOINT" default:"localhost:9000"`
	Bucket        string `env:"STORAGE_BUCKET" default:"listing-images"`
	AccessKey     string `env:"STORAGE_ACCESS_KEY" default:"minioadmin"`
	SecretKey     string `env:"STORAGE_SECRET_KEY" default:"minioadmin"`
	Region        string `env:"STORAGE_REGION" default:"us-east-1"`
	UseSSL        bool   `env:"STORAGE_USE_SSL" default:"false"`
	PublicBaseURL string `env:"STORAGE_PUBLIC_BASE_URL" default:"http://localhost:9000/listing-images"`
}

// Events names the topic the outbox relayer publishes ListingChanged and
// ListingStockChanged to (ADR-0002). The brokers come from the SDK Kafka group
// (KAFKA_BROKERS) and the relayer tuning from the SDK Outbox group
// (OUTBOX_POLL_INTERVAL, OUTBOX_BATCH_SIZE, OUTBOX_CLAIM_LOCK). The producer and
// the relayer are always on.
type Events struct {
	ListingTopic string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

// Reservation tunes stock-reservation timing (spec inventory-reservations).
// Both values are Go durations. A missing, unparsable or non-positive value
// falls back to the default with a WARN naming the variable (see ReservationTTL
// / ReservationSweepInterval) and never stops the service from starting: a typo
// must neither take the service down nor disable expiry.
type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m" keepempty:"true"`           // how long an uncommitted (active) reservation holds stock
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m" keepempty:"true"` // how often the sweeper restores expired active reservations
}

// Defaults for the reservation knobs; they must match the struct tags above.
const (
	DefaultReservationTTL           = 15 * time.Minute
	DefaultReservationSweepInterval = time.Minute
)

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the single entry point used by cmd/server. The reservation durations keep
// an explicitly empty value (keepempty), so it reaches positiveDuration, which
// falls back with a WARN naming the variable.
func Load() (Settings, error) {
	return sdkconfig.Load[Settings]()
}

// Validate enforces the checks the SDK groups do not make (a port above 65535, a
// negative shutdown grace) and prod-safety, mirroring team-ai's @model_validator
// + validate_core_resource_requirements. sdkconfig.Load calls it after parsing,
// next to the Validate of every SDK group (ENV, GRPC_PORT, DATABASE_URL).
func (s Settings) Validate() error {
	if s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	if s.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE must be >= 0: %v", s.ShutdownGrace)
	}
	return s.RequireSafeStrictConfig()
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	return e == "prod" || e == "production"
}

// RequireSafeStrictConfig is the boot guard against silent unsafe fallbacks. In a
// strict ENV (staging/stage/prod/production) it refuses config where:
//   - STORAGE_ACCESS_KEY / STORAGE_SECRET_KEY are still the minioadmin defaults.
//
// The database is required in every ENV, but that is enforced by the SDK Postgres
// group's Validate, not here. Other environments always pass.
func (s Settings) RequireSafeStrictConfig() error {
	if !s.IsStrict() {
		return nil
	}
	var bad []string
	if s.Storage.AccessKey == "minioadmin" || s.Storage.SecretKey == "minioadmin" {
		bad = append(bad, "STORAGE_ACCESS_KEY/STORAGE_SECRET_KEY are the insecure minioadmin defaults")
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start with unsafe config: ENV=%q is strict (ENV in staging, stage, prod, production) but %s",
		s.Env, strings.Join(bad, "; "))
}

// ReservationTTL parses RESERVATION_TTL. A missing, unparsable or non-positive
// value falls back to DefaultReservationTTL (15m) and logs a warning on logger
// (slog.Default() when nil).
func (s Settings) ReservationTTL(logger *slog.Logger) time.Duration {
	return positiveDuration(logger, "RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

// ReservationSweepInterval parses RESERVATION_SWEEP_INTERVAL. A missing,
// unparsable or non-positive value falls back to DefaultReservationSweepInterval
// (1m) and logs a warning on logger (slog.Default() when nil).
func (s Settings) ReservationSweepInterval(logger *slog.Logger) time.Duration {
	return positiveDuration(logger, "RESERVATION_SWEEP_INTERVAL", s.Reservation.SweepInterval, DefaultReservationSweepInterval)
}

// positiveDuration parses raw with sdkconfig.PositiveDuration; a blank, unparsable
// or non-positive value yields def plus a WARN naming key. Durations are logged
// as strings ("15m0s") so the JSON and text handlers render them the same way.
func positiveDuration(logger *slog.Logger, key, raw string, def time.Duration) time.Duration {
	d, err := sdkconfig.PositiveDuration(key, raw, def)
	if err == nil && strings.TrimSpace(raw) != "" {
		return d
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("invalid "+key+"; using the default",
		"key", key, "value", raw, "default", def.String())
	return def
}
