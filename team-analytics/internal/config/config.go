// Package config loads team-analytics settings from the environment through
// the go-sdk config groups; the env/default struct tags are the single source
// of truth for loading and for the .env.example drift gate.
package config

import (
	"errors"
	"fmt"
	"strings"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

const serviceName = "team-analytics"

// Warehouse driver identifiers selected by WAREHOUSE_DRIVER.
const (
	DriverDuckDB   = "duckdb"
	DriverBigQuery = "bigquery"
)

// Settings is the whole configuration surface.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Kafka      Kafka
	LogKeyFP   sdkconfig.LogKeyFP
	MockData   sdkconfig.MockData
	Metrics    Metrics
	Warehouse  Warehouse
	Batch      Batch
	Tracking   Tracking
	Recs       Recs
	Engagement Engagement
	GA4        GA4
	OTel       sdkconfig.OTel
}

// Metrics configures the HTTP listener of GET /metrics (empty disables it).
type Metrics struct {
	Addr string `env:"METRICS_ADDR" default:":9109" keepempty:"true"`
}

// Kafka configures the analytics-events, order and listing consumers.
type Kafka struct {
	Kafka                sdkconfig.Kafka
	ConsumerGroup        string `env:"KAFKA_CONSUMER_GROUP" default:"team-analytics"`
	AnalyticsTopic       string `env:"KAFKA_ANALYTICS_TOPIC" default:"analytics.events"`
	OrderTopic           string `env:"KAFKA_ORDER_TOPIC" default:"order.events"`
	DLQTopic             string `env:"KAFKA_DLQ_TOPIC" default:"analytics.events.dlq"`
	ListingTopic         string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
	ListingConsumerGroup string `env:"KAFKA_LISTING_CONSUMER_GROUP" default:"team-analytics-listing-attrs"`
	ListingDLQTopic      string `env:"KAFKA_LISTING_DLQ_TOPIC" default:"listing.events.analytics.dlq"`
}

// Engagement configures the engagement.events consumer; undecodable or unknown
// records are republished to DLQTopic and the consumer commits past them.
type Engagement struct {
	EventsTopic   string `env:"ENGAGEMENT_EVENTS_TOPIC" default:"engagement.events"`
	DLQTopic      string `env:"ENGAGEMENT_DLQ_TOPIC" default:"engagement.events.analytics.dlq"`
	ConsumerGroup string `env:"ENGAGEMENT_CONSUMER_GROUP" default:"team-analytics.engagement"`
}

// Warehouse selects and configures the WarehouseWriter adapter.
type Warehouse struct {
	Driver string `env:"WAREHOUSE_DRIVER" default:"duckdb"`

	DuckDBPath string `env:"DUCKDB_PATH" default:"/data/analytics.duckdb"`

	BigQueryProject string `env:"BIGQUERY_PROJECT" default:""`
	BigQueryDataset string `env:"BIGQUERY_DATASET" default:"analytics"`
	BigQueryTable   string `env:"BIGQUERY_TABLE" default:"tracking_events"`

	ParquetExportPath            string `env:"PARQUET_EXPORT_PATH" default:""`
	ParquetExportIntervalSeconds int    `env:"PARQUET_EXPORT_INTERVAL_SECONDS" default:"0"`
}

// Batch bounds the accumulate-then-flush loop.
type Batch struct {
	MaxSize              int `env:"BATCH_MAX_SIZE" default:"500"`
	FlushIntervalSeconds int `env:"BATCH_FLUSH_INTERVAL_SECONDS" default:"2"`
}

// Tracking holds the thresholds GetTrackingQualityReport derives its status from.
type Tracking struct {
	StaleAfterSeconds      int     `env:"TRACKING_STALE_AFTER_SECONDS" default:"900"`
	LagP95MaxSeconds       int     `env:"TRACKING_LAG_P95_MAX_SECONDS" default:"300"`
	MissingListingMaxRatio float64 `env:"TRACKING_MISSING_LISTING_MAX_RATIO" default:"0.05"`
}

// Recs configures recommendation outcome attribution.
type Recs struct {
	AttributionWindowHours int `env:"RECS_ATTRIBUTION_WINDOW_HOURS" default:"24"`
}

// GA4 configures the daily GA4 export import and the purchase relay.
type GA4 struct {
	ExportDataset         string  `env:"GA4_EXPORT_DATASET" default:""`
	FixtureDir            string  `env:"GA4_EXPORT_FIXTURE_DIR" default:""`
	ImportAt              string  `env:"GA4_IMPORT_AT" default:"03:30"`
	ImportTriggerURL      string  `env:"GA4_IMPORT_TRIGGER_URL" default:"http://localhost:9109/internal/ga4import"`
	ReconcileMaxDiffRatio float64 `env:"GA4_RECONCILE_MAX_DIFF_RATIO" default:"0.35"`

	APISecret          string `env:"GA4_API_SECRET" default:""`
	MeasurementID      string `env:"GA4_MEASUREMENT_ID" default:""`
	Endpoint           string `env:"GA4_ENDPOINT" default:"https://www.google-analytics.com/mp/collect"`
	RelayConsumerGroup string `env:"GA4_RELAY_CONSUMER_GROUP" default:"team-analytics.ga4-relay"`
	RelayDLQTopic      string `env:"GA4_RELAY_DLQ_TOPIC" default:"analytics.ga4-relay.dlq"`
}

// Load reads the environment into Settings and validates it.
func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	if s.OTel.ServiceName == "" {
		s.OTel.ServiceName = serviceName
	}
	return s, nil
}

// Validate enforces cross-field invariants and prod-safety.
func (s Settings) Validate() error {
	if s.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Port)
	}
	if s.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE must be >= 0: %v", s.ShutdownGrace)
	}
	if s.DrainDelay < 0 {
		return fmt.Errorf("SHUTDOWN_DRAIN_DELAY must be >= 0: %v", s.DrainDelay)
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
	if s.IsStrict() {
		var missing []string
		if strings.TrimSpace(s.GA4.APISecret) == "" {
			missing = append(missing, "GA4_API_SECRET")
		}
		if strings.TrimSpace(s.GA4.MeasurementID) == "" {
			missing = append(missing, "GA4_MEASUREMENT_ID")
		}
		if strings.TrimSpace(s.GA4.ExportDataset) == "" {
			missing = append(missing, "GA4_EXPORT_DATASET")
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s required when ENV is staging or prod", strings.Join(missing, ", "))
		}
	}
	if s.GA4RelayEnabled() {
		if strings.TrimSpace(s.GA4.Endpoint) == "" || strings.TrimSpace(s.GA4.RelayDLQTopic) == "" ||
			strings.TrimSpace(s.GA4.RelayConsumerGroup) == "" {
			return errors.New("GA4_ENDPOINT, GA4_RELAY_DLQ_TOPIC and GA4_RELAY_CONSUMER_GROUP must not be empty when the GA4 relay is enabled")
		}
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
