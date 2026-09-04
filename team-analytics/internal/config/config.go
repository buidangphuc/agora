// Package config assembles one flat Settings value from per-capability struct
// groups, populated from the environment with defaults — mirroring team-search's
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

// Warehouse driver identifiers selected by WAREHOUSE_DRIVER.
const (
	DriverDuckDB   = "duckdb"
	DriverBigQuery = "bigquery"
)

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	Runtime       Runtime
	Server        Server
	Kafka         Kafka
	Warehouse     Warehouse
	Batch         Batch
	Observability Observability
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// Server is the gRPC HEALTH server only (k8s probes). This worker serves no
// business RPC/HTTP — its input is the Kafka topic.
type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50059"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Kafka configures the analytics-events consumer (ADR-0002).
type Kafka struct {
	Enabled       bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers       string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-analytics"`
	AnalyticsTopic string `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
}

// Warehouse selects and configures the WarehouseWriter adapter. DuckDB is the
// local/test default (zero external deps, columnar Parquet); BigQuery is prod.
type Warehouse struct {
	Driver string `env:"WAREHOUSE_DRIVER" default:"duckdb"` // duckdb | bigquery

	// DuckDB adapter: filesystem path to the database/Parquet store.
	DuckDBPath string `env:"DUCKDB_PATH" default:"/data/analytics.duckdb"`

	// BigQuery adapter: project/dataset/table the rows are streamed into.
	BigQueryProject string `env:"BIGQUERY_PROJECT" default:""`
	BigQueryDataset string `env:"BIGQUERY_DATASET" default:"analytics"`
	BigQueryTable   string `env:"BIGQUERY_TABLE" default:"tracking_events"`
}

// Batch bounds the accumulate→flush loop: flush on whichever comes first, batch
// size or max interval (design.md, ~100 evts/s target). Offsets are committed
// only after a successful flush (at-least-once).
type Batch struct {
	MaxSize              int `env:"BATCH_MAX_SIZE" default:"500"`
	FlushIntervalSeconds int `env:"BATCH_FLUSH_INTERVAL_SECONDS" default:"2"`
}

// Observability configures OpenTelemetry (ADR-0004). Exporter swappable.
type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-analytics"`
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
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.Server.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.Server.ShutdownGrace)
	}
	if s.Batch.MaxSize <= 0 {
		return fmt.Errorf("BATCH_MAX_SIZE must be > 0: %d", s.Batch.MaxSize)
	}
	if s.Batch.FlushIntervalSeconds < 0 {
		return fmt.Errorf("BATCH_FLUSH_INTERVAL_SECONDS must be >= 0: %d", s.Batch.FlushIntervalSeconds)
	}
	switch s.Warehouse.Driver {
	case DriverDuckDB:
		if strings.TrimSpace(s.Warehouse.DuckDBPath) == "" {
			return errors.New("DUCKDB_PATH is required when WAREHOUSE_DRIVER=duckdb")
		}
	case DriverBigQuery:
		if strings.TrimSpace(s.Warehouse.BigQueryProject) == "" ||
			strings.TrimSpace(s.Warehouse.BigQueryDataset) == "" ||
			strings.TrimSpace(s.Warehouse.BigQueryTable) == "" {
			return errors.New("BIGQUERY_PROJECT, BIGQUERY_DATASET and BIGQUERY_TABLE are required when WAREHOUSE_DRIVER=bigquery")
		}
	default:
		return fmt.Errorf("WAREHOUSE_DRIVER must be %q or %q, got %q", DriverDuckDB, DriverBigQuery, s.Warehouse.Driver)
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
