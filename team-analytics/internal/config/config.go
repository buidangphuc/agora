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
	Tracking      Tracking
	Recs          Recs
	Engagement    Engagement
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
	Enabled        bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers        string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ConsumerGroup  string `env:"KAFKA_CONSUMER_GROUP" default:"team-analytics"`
	AnalyticsTopic string `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
	OrderTopic     string `env:"KAFKA_ORDER_TOPIC" default:"order.events"`
	// ListingTopic feeds the listing -> seller / category / price table (ListingChanged). It is
	// read by its own consumer group, from the earliest offset, so existing listings backfill.
	// The group name was bumped from team-analytics-listing-sellers when category and price
	// were added (featurestore-item-attributes): a new group replays the topic once and
	// refreshes every row.
	ListingTopic         string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
	ListingConsumerGroup string `env:"KAFKA_LISTING_CONSUMER_GROUP" default:"team-analytics-listing-attrs"`
}

// Engagement configures the engagement.events consumer (engagement-fact-events D3).
// It runs in its own consumer group; undecodable or unknown records are
// republished to DLQTopic and the consumer commits past them.
type Engagement struct {
	EventsTopic   string `env:"ENGAGEMENT_EVENTS_TOPIC" default:"engagement.events"`
	DLQTopic      string `env:"ENGAGEMENT_DLQ_TOPIC" default:"engagement.events.analytics.dlq"`
	ConsumerGroup string `env:"ENGAGEMENT_CONSUMER_GROUP" default:"team-analytics.engagement"`
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

	// Offline-training export (DuckDB only): every ParquetExportIntervalSeconds the
	// tracking_events table is written to ParquetExportPath (atomic replace) for
	// platform-recsys. 0 disables it.
	ParquetExportPath            string `env:"PARQUET_EXPORT_PATH" default:""`
	ParquetExportIntervalSeconds int    `env:"PARQUET_EXPORT_INTERVAL_SECONDS" default:"0"`
}

// Batch bounds the accumulate→flush loop: flush on whichever comes first, batch
// size or max interval (design.md, ~100 evts/s target). Offsets are committed
// only after a successful flush (at-least-once).
type Batch struct {
	MaxSize              int `env:"BATCH_MAX_SIZE" default:"500"`
	FlushIntervalSeconds int `env:"BATCH_FLUSH_INTERVAL_SECONDS" default:"2"`
}

// Tracking holds the thresholds GetTrackingQualityReport derives its status from
// (analytics-data-quality D2).
type Tracking struct {
	// StaleAfterSeconds: the report is DEGRADED(stale) when the latest ingest is older.
	StaleAfterSeconds int `env:"TRACKING_STALE_AFTER_SECONDS" default:"900"`
	// LagP95MaxSeconds: DEGRADED(lagging) when the p95 ingest lag exceeds it.
	LagP95MaxSeconds int `env:"TRACKING_LAG_P95_MAX_SECONDS" default:"300"`
	// MissingListingMaxRatio: DEGRADED(incomplete) when a listing-scoped event
	// type's share of events without listing_id exceeds it (0 to 1).
	MissingListingMaxRatio float64 `env:"TRACKING_MISSING_LISTING_MAX_RATIO" default:"0.05"`
}

// Recs configures recommendation outcome attribution (recsys-online-evaluation).
type Recs struct {
	// AttributionWindowHours: an add-to-cart or purchase is credited to a click
	// when it happens within this many hours after it.
	AttributionWindowHours int `env:"RECS_ATTRIBUTION_WINDOW_HOURS" default:"24"`
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
	if s.Warehouse.ParquetExportIntervalSeconds < 0 {
		return fmt.Errorf("PARQUET_EXPORT_INTERVAL_SECONDS must be >= 0: %d", s.Warehouse.ParquetExportIntervalSeconds)
	}
	if s.Tracking.StaleAfterSeconds <= 0 {
		return fmt.Errorf("TRACKING_STALE_AFTER_SECONDS must be > 0: %d", s.Tracking.StaleAfterSeconds)
	}
	if s.Tracking.LagP95MaxSeconds <= 0 {
		return fmt.Errorf("TRACKING_LAG_P95_MAX_SECONDS must be > 0: %d", s.Tracking.LagP95MaxSeconds)
	}
	if s.Tracking.MissingListingMaxRatio < 0 || s.Tracking.MissingListingMaxRatio > 1 {
		return fmt.Errorf("TRACKING_MISSING_LISTING_MAX_RATIO must be within [0, 1]: %v", s.Tracking.MissingListingMaxRatio)
	}
	if s.Recs.AttributionWindowHours <= 0 {
		return fmt.Errorf("RECS_ATTRIBUTION_WINDOW_HOURS must be > 0: %d", s.Recs.AttributionWindowHours)
	}
	if strings.TrimSpace(s.Engagement.EventsTopic) == "" || strings.TrimSpace(s.Engagement.DLQTopic) == "" ||
		strings.TrimSpace(s.Engagement.ConsumerGroup) == "" {
		return errors.New("ENGAGEMENT_EVENTS_TOPIC, ENGAGEMENT_DLQ_TOPIC and ENGAGEMENT_CONSUMER_GROUP must not be empty")
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
