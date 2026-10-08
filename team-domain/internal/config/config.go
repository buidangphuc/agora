// Package config assembles one flat Settings value from per-capability struct
// groups, populated from the environment with defaults — the Go analogue of
// team-ai's pydantic Settings (mixins + .env + prod-safety validators).
//
// Grouping is only a file-organization device: fields are read from flat env
// keys (GRPC_PORT, DATABASE_URL, ...). The `env`/`default` struct tags are the
// single source of truth for both loading (LoadSettings) and the .env.example
// drift gate (envcheck.go), so the two can never disagree.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	Runtime       Runtime
	Server        Server
	Database      Database
	Storage       Storage
	Events        Events
	Outbox        Outbox
	Reservation   Reservation
	Observability Observability
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

// Runtime holds process-wide runtime knobs.
type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// Server configures the gRPC listener and graceful shutdown.
type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50051"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Database configures this service's OWN Postgres (Rule 3 / DB-per-service).
type Database struct {
	Enabled  bool   `env:"DATABASE_ENABLED" default:"true"`
	URL      string `env:"DATABASE_URL" default:""`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

// Events configures the Kafka producer that emits ListingChanged on every write
// (ADR-0002). When KafkaEnabled is false the producer is a no-op.
type Events struct {
	KafkaEnabled bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers      string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ListingTopic string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

// Outbox configures the transactional-outbox relayer (ADR-0002). Listing writes
// always record an outbox row in the same DB transaction; this group tunes the
// background relayer that publishes those rows to Kafka. The relayer only runs
// when OUTBOX_ENABLED and KAFKA_ENABLED are both true — with Kafka disabled,
// rows are recorded but never relayed (matching the prior no-emit behaviour).
type Outbox struct {
	Enabled          bool   `env:"OUTBOX_ENABLED" default:"true"`
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

// Observability configures OpenTelemetry (ADR-0004). Exporter is swappable.
type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-domain"`
}

// LoadSettings reads the environment into a Settings value, applies defaults,
// and runs Validate. It is the single entry point used by the server entrypoint.
func LoadSettings() (*Settings, error) {
	s := &Settings{}
	if err := bindGroups(reflect.ValueOf(s).Elem()); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate enforces cross-field invariants and prod-safety, mirroring team-ai's
// @model_validator + validate_core_resource_requirements.
func (s *Settings) Validate() error {
	if s.Database.Enabled && strings.TrimSpace(s.Database.URL) == "" {
		return errors.New("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.Server.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.Server.ShutdownGrace)
	}
	return s.RequireSafeStrictConfig()
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

// strictEnvs are the ENV values (normalised: trimmed, lowercase) in which unsafe
// fallbacks are refused at boot. Anything else ("local", "test", unknown) is
// non-strict.
var strictEnvs = []string{"staging", "stage", "prod", "production"}

// IsStrictEnv reports whether ENV names staging or production.
func (s *Settings) IsStrictEnv() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	for _, strict := range strictEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

// RequireSafeStrictConfig is the boot guard against silent unsafe fallbacks. In a
// strict ENV (staging/stage/prod/production) it refuses config where:
//   - KAFKA_ENABLED=false: the no-op publisher is used, so outbox rows are recorded
//     but never relayed and read-models (team-search) silently go stale;
//   - OUTBOX_ENABLED=false: the relayer never runs, same effect;
//   - STORAGE_ACCESS_KEY / STORAGE_SECRET_KEY are still the minioadmin defaults.
//
// The database is required in every ENV, but that is enforced by Validate's
// DATABASE_ENABLED/URL check and bootstrap, not here. Other environments always pass.
func (s *Settings) RequireSafeStrictConfig() error {
	if !s.IsStrictEnv() {
		return nil
	}
	var bad []string
	if !s.Events.KafkaEnabled {
		bad = append(bad, "KAFKA_ENABLED=false (the no-op publisher is used: outbox rows would never be relayed)")
	}
	if !s.Outbox.Enabled {
		bad = append(bad, "OUTBOX_ENABLED=false (the outbox relayer would not run)")
	}
	if s.Storage.AccessKey == "minioadmin" || s.Storage.SecretKey == "minioadmin" {
		bad = append(bad, "STORAGE_ACCESS_KEY/STORAGE_SECRET_KEY are the insecure minioadmin defaults")
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start with unsafe config: ENV=%q is strict (ENV in %s) but %s",
		s.Runtime.Env, strings.Join(strictEnvs, ", "), strings.Join(bad, "; "))
}

// KafkaBrokers splits the comma-separated KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string {
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
func (s *Settings) OutboxPollInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s.Outbox.PollInterval))
	if err != nil || d <= 0 {
		return time.Second
	}
	return d
}

// DeclaredEnvKeys returns every env key declared by Settings, in struct order.
// The .env.example drift gate (envcheck.go) compares against this set.
func DeclaredEnvKeys() []string {
	var keys []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		gt := t.Field(i).Type
		for j := 0; j < gt.NumField(); j++ {
			if k := gt.Field(j).Tag.Get("env"); k != "" {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// bindGroups walks each capability group and binds its fields from env/default.
func bindGroups(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		group := v.Field(i)
		gt := group.Type()
		for j := 0; j < gt.NumField(); j++ {
			f := gt.Field(j)
			key := f.Tag.Get("env")
			if key == "" {
				continue
			}
			raw := f.Tag.Get("default")
			if val, ok := os.LookupEnv(key); ok {
				raw = val
			}
			if err := setField(group.Field(j), raw); err != nil {
				return fmt.Errorf("config %s: %w", key, err)
			}
		}
	}
	return nil
}

// setField parses raw into fv according to its kind.
func setField(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		x, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return err
		}
		fv.SetFloat(x)
	default:
		return fmt.Errorf("unsupported config field kind %s", fv.Kind())
	}
	return nil
}

// ReservationTTL parses RESERVATION_TTL. A missing, unparsable or non-positive
// value falls back to DefaultReservationTTL (15m) and logs a warning on logger
// (slog.Default() when nil).
func (s *Settings) ReservationTTL(logger *slog.Logger) time.Duration {
	return positiveDuration(logger, "RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

// ReservationSweepInterval parses RESERVATION_SWEEP_INTERVAL. A missing,
// unparsable or non-positive value falls back to DefaultReservationSweepInterval
// (1m) and logs a warning on logger (slog.Default() when nil).
func (s *Settings) ReservationSweepInterval(logger *slog.Logger) time.Duration {
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
		slog.String("key", key), slog.String("value", raw), slog.String("default", def.String()))
	return def
}
