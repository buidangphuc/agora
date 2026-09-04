package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/config"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`).
func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := config.CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50059 {
		t.Errorf("default GRPC_PORT = %d, want 50059", s.Server.Port)
	}
	if s.Kafka.AnalyticsTopic != "analytics.events" {
		t.Errorf("default KAFKA_ANALYTICS_TOPIC = %q, want analytics.events", s.Kafka.AnalyticsTopic)
	}
	if s.Warehouse.Driver != config.DriverDuckDB {
		t.Errorf("default WAREHOUSE_DRIVER = %q, want duckdb", s.Warehouse.Driver)
	}
	if s.Batch.MaxSize != 500 {
		t.Errorf("default BATCH_MAX_SIZE = %d, want 500", s.Batch.MaxSize)
	}
}

func TestValidateRejectsUnknownDriver(t *testing.T) {
	s := &config.Settings{}
	s.Server.Port = 50059
	s.Batch.MaxSize = 1
	s.Warehouse.Driver = "sqlite"
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: unknown WAREHOUSE_DRIVER")
	}
}

func TestValidateBigQueryRequiresTarget(t *testing.T) {
	s := &config.Settings{}
	s.Server.Port = 50059
	s.Batch.MaxSize = 1
	s.Warehouse.Driver = config.DriverBigQuery
	s.Warehouse.BigQueryDataset = "analytics"
	s.Warehouse.BigQueryTable = "tracking_events"
	// BigQueryProject intentionally empty.
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: BIGQUERY_PROJECT required for bigquery driver")
	}
}
