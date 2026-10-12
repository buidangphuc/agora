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

// ---- add-ga4-purchase-relay / cfg: requirement "Disabled without secret" ----

func clearRelayEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GA4_API_SECRET", "GA4_MEASUREMENT_ID", "GA4_ENDPOINT",
		"GA4_RELAY_CONSUMER_GROUP", "GA4_RELAY_DLQ_TOPIC", "GA4_RELAY_PROBE_PORT"} {
		t.Setenv(k, "") // registers restore of the original value
		os.Unsetenv(k)  // truly unset, so empty-string semantics are not conflated with unset
	}
}

// Scenario: Disabled without secret (an explicitly empty string is honoured, not replaced by the default).
func TestGA4RelayExplicitEmptyEndpointIsHonoured_DisabledWithoutSecret(t *testing.T) {
	clearRelayEnv(t)
	t.Setenv("GA4_ENDPOINT", "")
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings (relay disabled): %v", err)
	}
	if s.GA4.Endpoint != "" {
		t.Errorf("explicit empty GA4_ENDPOINT = %q, want empty (no default substitution)", s.GA4.Endpoint)
	}
}

// Scenario: Disabled without secret (defaults: nothing set -> disabled, MP URL, DLQ, probe port).
func TestGA4RelayDefaults_DisabledWithoutSecret(t *testing.T) {
	clearRelayEnv(t)
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.GA4.APISecret != "" || s.GA4.MeasurementID != "" {
		t.Errorf("secret/measurement id must default empty, got %q / %q", s.GA4.APISecret, s.GA4.MeasurementID)
	}
	if s.GA4RelayEnabled() {
		t.Error("relay must be disabled when GA4_API_SECRET and GA4_MEASUREMENT_ID are unset")
	}
	if want := "https://www.google-analytics.com/mp/collect"; s.GA4.Endpoint != want {
		t.Errorf("GA4_ENDPOINT default = %q, want %q", s.GA4.Endpoint, want)
	}
	if s.GA4.RelayDLQTopic != "analytics.ga4-relay.dlq" {
		t.Errorf("GA4_RELAY_DLQ_TOPIC default = %q, want analytics.ga4-relay.dlq", s.GA4.RelayDLQTopic)
	}
	if s.GA4.RelayConsumerGroup == "" {
		t.Error("GA4_RELAY_CONSUMER_GROUP must have a non-empty default")
	}
	if s.GA4.RelayConsumerGroup == s.Kafka.ConsumerGroup || s.GA4.RelayConsumerGroup == s.Engagement.ConsumerGroup ||
		s.GA4.RelayConsumerGroup == s.Kafka.ListingConsumerGroup {
		t.Errorf("relay consumer group %q must be distinct from the other groups", s.GA4.RelayConsumerGroup)
	}
	if s.GA4.RelayProbePort != 8085 {
		t.Errorf("GA4_RELAY_PROBE_PORT default = %d, want 8085 (design Q3)", s.GA4.RelayProbePort)
	}
}

// Scenario: Disabled without secret (each missing half disables; both present enables).
func TestGA4RelayEnabled_RequiresSecretAndMeasurementID_DisabledWithoutSecret(t *testing.T) {
	cases := []struct {
		name, secret, mid string
		want              bool
	}{
		{"both unset", "", "", false},
		{"secret only", "s3cret", "", false},
		{"measurement id only", "", "G-ABC123", false},
		{"whitespace secret", "   ", "G-ABC123", false},
		{"both set", "s3cret", "G-ABC123", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearRelayEnv(t)
			t.Setenv("GA4_API_SECRET", c.secret)
			t.Setenv("GA4_MEASUREMENT_ID", c.mid)
			s, err := config.LoadSettings()
			if err != nil {
				t.Fatalf("LoadSettings must succeed even when the relay is disabled: %v", err)
			}
			if got := s.GA4RelayEnabled(); got != c.want {
				t.Errorf("GA4RelayEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}

// Scenario: No Google traffic in e2e (GA4_ENDPOINT overridable) + probe port override (design Q3).
func TestGA4RelayEnvOverrides_DisabledWithoutSecret(t *testing.T) {
	clearRelayEnv(t)
	t.Setenv("GA4_API_SECRET", "s3cret")
	t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
	t.Setenv("GA4_ENDPOINT", "http://ga4-mock:8080/mp/collect")
	t.Setenv("GA4_RELAY_DLQ_TOPIC", "custom.dlq")
	t.Setenv("GA4_RELAY_CONSUMER_GROUP", "custom-group")
	t.Setenv("GA4_RELAY_PROBE_PORT", "9090")
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.GA4.APISecret != "s3cret" || s.GA4.MeasurementID != "G-ABC123" {
		t.Errorf("secret/measurement id not loaded: %q / %q", s.GA4.APISecret, s.GA4.MeasurementID)
	}
	if s.GA4.Endpoint != "http://ga4-mock:8080/mp/collect" || s.GA4.RelayDLQTopic != "custom.dlq" ||
		s.GA4.RelayConsumerGroup != "custom-group" || s.GA4.RelayProbePort != 9090 {
		t.Errorf("overrides not applied: %+v", s.GA4)
	}
}

// Scenario: Disabled without secret (enabled relay needs a usable endpoint/DLQ/group/port).
func TestGA4RelayValidate_EnabledRequiresUsableConfig_DisabledWithoutSecret(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"empty endpoint": {"GA4_ENDPOINT": " "},
		"empty dlq":      {"GA4_RELAY_DLQ_TOPIC": " "},
		"empty group":    {"GA4_RELAY_CONSUMER_GROUP": " "},
		"port too high":  {"GA4_RELAY_PROBE_PORT": "70000"},
		"port zero":      {"GA4_RELAY_PROBE_PORT": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			clearRelayEnv(t)
			t.Setenv("GA4_API_SECRET", "s3cret")
			t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := config.LoadSettings(); err == nil {
				t.Fatal("expected validation error for an enabled relay with unusable config")
			}
		})
	}
}

// Scenario: Secret never logged (secret must not leak via error text from Validate).
func TestGA4RelayInvalidConfigErrorDoesNotContainSecret(t *testing.T) {
	clearRelayEnv(t)
	t.Setenv("GA4_API_SECRET", "TOP-SECRET-VALUE")
	t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
	t.Setenv("GA4_RELAY_PROBE_PORT", "70000")
	_, err := config.LoadSettings()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "TOP-SECRET-VALUE") {
		t.Errorf("error leaks the API secret: %v", err)
	}
}
