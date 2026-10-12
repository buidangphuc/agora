package config_test

import (
	"os"
	"strings"
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

func TestTrackingDefaultsAndValidation(t *testing.T) {
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Tracking.StaleAfterSeconds != 900 || s.Tracking.LagP95MaxSeconds != 300 || s.Tracking.MissingListingMaxRatio != 0.05 {
		t.Errorf("tracking defaults = %+v, want 900/300/0.05", s.Tracking)
	}
	for _, mut := range []func(*config.Settings){
		func(s *config.Settings) { s.Tracking.StaleAfterSeconds = 0 },
		func(s *config.Settings) { s.Tracking.LagP95MaxSeconds = 0 },
		func(s *config.Settings) { s.Tracking.MissingListingMaxRatio = 1.1 },
		func(s *config.Settings) { s.Tracking.MissingListingMaxRatio = -0.1 },
	} {
		bad := *s
		mut(&bad)
		if err := bad.Validate(); err == nil {
			t.Errorf("Validate accepted %+v", bad.Tracking)
		}
	}
}

func TestRecsAttributionDefaultAndValidation(t *testing.T) {
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Recs.AttributionWindowHours != 24 {
		t.Errorf("attribution window default = %d, want 24", s.Recs.AttributionWindowHours)
	}
	bad := *s
	bad.Recs.AttributionWindowHours = 0
	if err := bad.Validate(); err == nil {
		t.Error("Validate accepted RECS_ATTRIBUTION_WINDOW_HOURS=0")
	}
}

// Scenario: Invalid threshold is rejected at startup
func TestInvalidGA4ReconcileThresholdIsRejectedAtStartup(t *testing.T) {
	for _, v := range []string{"-1", "abc"} {
		t.Setenv("GA4_RECONCILE_MAX_DIFF_RATIO", v)
		_, err := config.LoadSettings()
		if err == nil || !strings.Contains(err.Error(), "GA4_RECONCILE_MAX_DIFF_RATIO") {
			t.Errorf("value %q: want error naming GA4_RECONCILE_MAX_DIFF_RATIO, got %v", v, err)
		}
	}
}

// Scenario: The job is a no-op when unconfigured (GA4 settings default to unset dataset, valid threshold)
func TestGA4SettingsDefaults(t *testing.T) {
	t.Setenv("GA4_EXPORT_DATASET", "")
	t.Setenv("GA4_RECONCILE_MAX_DIFF_RATIO", "")
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.GA4.ExportDataset != "" {
		t.Errorf("GA4_EXPORT_DATASET default = %q, want empty", s.GA4.ExportDataset)
	}
	if s.GA4.ReconcileMaxDiffRatio != 0.35 {
		t.Errorf("GA4_RECONCILE_MAX_DIFF_RATIO default = %v, want 0.35", s.GA4.ReconcileMaxDiffRatio)
	}
}
