// Package config assembles one flat Settings value from per-capability struct
// groups, populated from the environment with defaults — mirroring team-domain's
// config (and, one step back, team-ai's pydantic Settings). The `env`/`default`
// struct tags are the single source of truth for loading AND the .env.example
// drift gate (envcheck.go).
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	Runtime       Runtime
	Server        Server
	OpenSearch    OpenSearch
	Kafka         Kafka
	Observability Observability
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50052"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// OpenSearch is this service's OWN read-model store (ADR-0005, Rule 3).
type OpenSearch struct {
	URL   string `env:"OPENSEARCH_URL" default:"http://localhost:9200"`
	Index string `env:"OPENSEARCH_INDEX" default:"listings"`
}

// Kafka configures the listing-events consumer (ADR-0002).
type Kafka struct {
	Enabled       bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers       string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-search-indexer"`
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

// Observability configures OpenTelemetry (ADR-0004). Exporter swappable.
type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-search"`
}

// LoadSettings reads the environment into Settings, applies defaults, validates.
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

// Validate enforces cross-field invariants and prod-safety.
func (s *Settings) Validate() error {
	if strings.TrimSpace(s.OpenSearch.URL) == "" {
		return errors.New("OPENSEARCH_URL is required")
	}
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.Server.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.Server.ShutdownGrace)
	}
	return nil
}

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

// KafkaBrokers splits KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string { return splitCSV(s.Kafka.Brokers) }

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// DeclaredEnvKeys returns every env key declared by Settings, in struct order.
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
