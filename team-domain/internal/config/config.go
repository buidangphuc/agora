// Package config holds team-domain's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel) plus the service-specific groups below, bound
// from flat env keys (GRPC_PORT, DATABASE_URL, ...) with sdkconfig.Load. The
// `env`/`default` struct tags are the single source of truth for both loading
// and the .env.example drift test (config_test.go).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	Storage     Storage
	Events      Events
	Outbox      Outbox
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

// Events configures the Kafka producer that emits ListingChanged on every write
// (ADR-0002). The producer is always on.
type Events struct {
	Brokers      string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ListingTopic string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

// Outbox configures the transactional-outbox relayer (ADR-0002). Listing writes
// always record an outbox row in the same DB transaction; this group tunes the
// background relayer that publishes those rows to Kafka. The relayer is always on.
type Outbox struct {
	PollInterval     string `env:"OUTBOX_POLL_INTERVAL" default:"1s"` // Go duration, e.g. 1s, 500ms
	BatchSize        int    `env:"OUTBOX_BATCH_SIZE" default:"100"`
	ClaimLockSeconds int    `env:"OUTBOX_CLAIM_LOCK_SECONDS" default:"60"`
	MaxAttempts      int    `env:"OUTBOX_MAX_ATTEMPTS" default:"10"`
}

// Reservation tunes stock-reservation timing (spec inventory-reservations).
// Both values are Go durations. A missing, unparsable or non-positive value
// falls back to the default with a WARN naming the variable (see ReservationTTL
// / ReservationSweepInterval) and never stops the service from starting: a typo
// must neither take the service down nor disable expiry.
type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m"`           // how long an uncommitted (active) reservation holds stock
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m"` // how often the sweeper restores expired active reservations
}

// Defaults for the reservation knobs; they must match the struct tags above.
const (
	DefaultReservationTTL           = 15 * time.Minute
	DefaultReservationSweepInterval = time.Minute
)

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the single entry point used by cmd/server.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	// The binder treats an empty value like an unset one and applies the default.
	// For the reservation durations an explicitly empty value must instead reach
	// positiveDuration, which falls back with a WARN naming the variable.
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

// Validate enforces cross-field invariants and prod-safety, mirroring team-ai's
// @model_validator + validate_core_resource_requirements. sdkconfig.Load calls
// it after parsing.
func (s Settings) Validate() error {
	if s.Postgres.Enabled && strings.TrimSpace(s.Postgres.URL) == "" {
		return errors.New("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	if s.ShutdownGraceSeconds < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.ShutdownGraceSeconds)
	}
	return s.RequireSafeStrictConfig()
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	return e == "prod" || e == "production"
}

// strictEnvs are the ENV values (normalised: trimmed, lowercase) in which unsafe
// fallbacks are refused at boot. Anything else ("local", "test", unknown) is
// non-strict.
var strictEnvs = []string{"staging", "stage", "prod", "production"}

// IsStrictEnv reports whether ENV names staging or production.
func (s Settings) IsStrictEnv() bool {
	e := strings.ToLower(strings.TrimSpace(s.Env))
	for _, strict := range strictEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

// RequireSafeStrictConfig is the boot guard against silent unsafe fallbacks. In a
// strict ENV (staging/stage/prod/production) it refuses config where:
//   - STORAGE_ACCESS_KEY / STORAGE_SECRET_KEY are still the minioadmin defaults.
//
// The database is required in every ENV, but that is enforced by Validate's
// DATABASE_ENABLED/URL check and bootstrap, not here. Other environments always pass.
func (s Settings) RequireSafeStrictConfig() error {
	if !s.IsStrictEnv() {
		return nil
	}
	var bad []string
	if s.Storage.AccessKey == "minioadmin" || s.Storage.SecretKey == "minioadmin" {
		bad = append(bad, "STORAGE_ACCESS_KEY/STORAGE_SECRET_KEY are the insecure minioadmin defaults")
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start with unsafe config: ENV=%q is strict (ENV in %s) but %s",
		s.Env, strings.Join(strictEnvs, ", "), strings.Join(bad, "; "))
}

// KafkaBrokers splits the comma-separated KAFKA_BROKERS into seed addresses.
func (s Settings) KafkaBrokers() []string {
	parts := strings.Split(s.Events.Brokers, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// OutboxPollInterval parses OUTBOX_POLL_INTERVAL into a duration, defaulting to
// 1s when unset or unparseable.
func (s Settings) OutboxPollInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s.Outbox.PollInterval))
	if err != nil || d <= 0 {
		return time.Second
	}
	return d
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

// positiveDuration parses raw as a Go duration; anything unusable yields def plus
// a WARN naming key. Durations are logged as strings ("15m0s") so the JSON and
// text handlers render them the same way.
func positiveDuration(logger *slog.Logger, key, raw string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err == nil && d > 0 {
		return d
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("invalid "+key+"; using the default",
		"key", key, "value", raw, "default", def.String())
	return def
}
