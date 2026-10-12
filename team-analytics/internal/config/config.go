// Package config holds team-analytics's settings: the SDK groups it uses plus
// its own groups, bound from environment variables. The `env`/`default` struct
// tags are the single source of truth for loading AND the .env.example drift
// gate (TestEnvExampleInSync).
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Warehouse driver identifiers selected by WAREHOUSE_DRIVER.
const (
	DriverDuckDB   = "duckdb"
	DriverBigQuery = "bigquery"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded
// because nothing else collides with their fields; the other groups are named.
// The gRPC server serves the read-only query API and the health probes; GRPC
// also carries SHUTDOWN_GRACE_SECONDS and SHUTDOWN_DRAIN_DELAY_SECONDS.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	OTel       sdkconfig.OTel
	Metrics    Metrics
	Kafka      Kafka
	Warehouse  Warehouse
	Batch      Batch
	Tracking   Tracking
	Recs       Recs
	Engagement Engagement
	GA4        GA4
}

// Metrics configures the HTTP listener of GET /metrics and the GA4 import trigger.
type Metrics struct {
	// Addr is the HTTP listen address (empty disables it).
	Addr string `env:"METRICS_ADDR" default:":9109"`
}

// Kafka configures the analytics-events consumer (ADR-0002).
type Kafka struct {
	sdkconfig.Kafka        // KAFKA_BROKERS
	ConsumerGroup   string `env:"KAFKA_CONSUMER_GROUP" default:"team-analytics"`
	AnalyticsTopic  string `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
	OrderTopic      string `env:"KAFKA_ORDER_TOPIC" default:"order.events"`
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

// GA4 configures the daily GA4 export import (run inside cmd/consumer). It is a no-op
// while ExportDataset is empty.
type GA4 struct {
	// ExportSource: bigquery (default when empty) | fixture (Parquet files for local/e2e).
	ExportSource string `env:"GA4_EXPORT_SOURCE" default:"bigquery"`
	// ExportDataset: BigQuery dataset holding events_YYYYMMDD ("dataset" or "project.dataset").
	ExportDataset string `env:"GA4_EXPORT_DATASET" default:""`
	// FixtureDir holds events_YYYYMMDD.parquet when ExportSource=fixture.
	FixtureDir string `env:"GA4_EXPORT_FIXTURE_DIR" default:""`
	// ImportAt: daily UTC "HH:MM" the consumer imports yesterday's export.
	ImportAt string `env:"GA4_IMPORT_AT" default:"03:30"`
	// ReconcileMaxDiffRatio: GA4 vs first-party event divergence threshold (>= 0).
	ReconcileMaxDiffRatio float64 `env:"GA4_RECONCILE_MAX_DIFF_RATIO" default:"0.35"`

	// Purchase relay (Measurement Protocol). Disabled unless APISecret and MeasurementID are both set.
	// APISecret is a secret: never log it or include it in errors.
	APISecret          string `env:"GA4_API_SECRET" default:""`
	MeasurementID      string `env:"GA4_MEASUREMENT_ID" default:""`
	Endpoint           string `env:"GA4_ENDPOINT" default:"https://www.google-analytics.com/mp/collect"`
	RelayConsumerGroup string `env:"GA4_RELAY_CONSUMER_GROUP" default:"team-analytics.ga4-relay"`
	RelayDLQTopic      string `env:"GA4_RELAY_DLQ_TOPIC" default:"analytics.ga4-relay.dlq"`
	RelayProbePort     int    `env:"GA4_RELAY_PROBE_PORT" default:"8085"`
}

// Validate enforces cross-field invariants and prod-safety.
func (s Settings) Validate() error {
	if err := s.GRPC.Validate(); err != nil {
		return err
	}
	if s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	if s.ShutdownGraceSeconds < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.ShutdownGraceSeconds)
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
	if s.GA4.ReconcileMaxDiffRatio < 0 {
		return fmt.Errorf("GA4_RECONCILE_MAX_DIFF_RATIO must be >= 0: %v", s.GA4.ReconcileMaxDiffRatio)
	}
	if s.GA4RelayEnabled() {
		if strings.TrimSpace(s.GA4.Endpoint) == "" || strings.TrimSpace(s.GA4.RelayDLQTopic) == "" ||
			strings.TrimSpace(s.GA4.RelayConsumerGroup) == "" {
			return errors.New("GA4_ENDPOINT, GA4_RELAY_DLQ_TOPIC and GA4_RELAY_CONSUMER_GROUP must not be empty when the GA4 relay is enabled")
		}
		if s.GA4.RelayProbePort <= 0 || s.GA4.RelayProbePort > 65535 {
			return fmt.Errorf("GA4_RELAY_PROBE_PORT out of range: %d", s.GA4.RelayProbePort)
		}
	}
	switch s.GA4.ExportSource {
	case "", "bigquery", "fixture":
	default:
		return fmt.Errorf("GA4_EXPORT_SOURCE must be %q or %q, got %q", "bigquery", "fixture", s.GA4.ExportSource)
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

// GA4RelayEnabled reports whether the purchase relay has its credentials.
func (s Settings) GA4RelayEnabled() bool {
	return strings.TrimSpace(s.GA4.APISecret) != "" && strings.TrimSpace(s.GA4.MeasurementID) != ""
}

// Load reads Settings from the environment and validates it.
//
// The SDK binder replaces a string variable that is set but empty with its
// default. This service has always honoured an explicit empty string (for
// example GA4_ENDPOINT= or DUCKDB_PATH=, which Validate then rejects), so Load
// restores it and validates again.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	honourExplicitEmpty(reflect.ValueOf(&s).Elem())
	if err := s.Validate(); err != nil {
		return s, err
	}
	return s, nil
}

// honourExplicitEmpty sets every string field whose env variable is set to ""
// back to "", recursing into nested groups.
func honourExplicitEmpty(v reflect.Value) {
	t := v.Type()
	for i := range t.NumField() {
		f, fv := t.Field(i), v.Field(i)
		if !f.IsExported() {
			continue
		}
		if tag, ok := f.Tag.Lookup("env"); ok {
			key, _, _ := strings.Cut(tag, ",")
			if val, set := os.LookupEnv(key); set && val == "" && fv.Kind() == reflect.String {
				fv.SetString("")
			}
			continue
		}
		if fv.Kind() == reflect.Struct {
			honourExplicitEmpty(fv)
		}
	}
}
