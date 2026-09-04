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
	return nil
}

// IsProd reports whether this is a production environment (case-insensitive).
func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
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
