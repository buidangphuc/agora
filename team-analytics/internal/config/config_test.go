package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-analytics/internal/config"
)

var legacyKeys = []string{
	"KAFKA_ENABLED", "SHUTDOWN_GRACE_SECONDS", "GA4_EXPORT_SOURCE", "GA4_RELAY_PROBE_PORT",
}

func cleanEnv(t *testing.T) {
	t.Helper()
	keys := append(sdkconfig.DeclaredEnvKeys(config.Settings{}), legacyKeys...)
	for _, k := range keys {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func localEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50059")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("GRPC_PORT", "50059")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
	t.Setenv("GA4_API_SECRET", "ga4-secret")
	t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
	t.Setenv("GA4_EXPORT_DATASET", "ga4_export")
}

func mustErrNaming(t *testing.T, err error, keys ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("load succeeded, want an error naming %v", keys)
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err.Error(), k)
		}
	}
}

func mustLoad(t *testing.T) config.Settings {
	t.Helper()
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// Scenario: .env.example documents exactly the declared keys
func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

// Scenario: ENV is required
func TestLoadRequiresENV(t *testing.T) {
	localEnv(t)
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")
}

// Scenario: ENV is required (GRPC_PORT has no default in the SDK group)
func TestLoadRequiresGRPCPort_ENVIsRequired(t *testing.T) {
	localEnv(t)
	if err := os.Unsetenv("GRPC_PORT"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_PORT")
}

// Scenario: A strict ENV requires the broker address
func TestStrictEnvRequiresKafkaBrokers(t *testing.T) {
	for _, env := range []string{"staging", "production"} {
		t.Run(env, func(t *testing.T) {
			strictEnv(t, env)
			if err := os.Unsetenv("KAFKA_BROKERS"); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load()
			mustErrNaming(t, err, "KAFKA_BROKERS")
		})
	}
}

// Scenario: A strict ENV requires the key fingerprint secret
func TestStrictEnvRequiresLogKeyFPSecret(t *testing.T) {
	strictEnv(t, "production")
	if err := os.Unsetenv("LOG_KEY_FP_SECRET"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "LOG_KEY_FP_SECRET")
}

// Scenario: A local ENV fills the defaults
func TestLocalEnvFillsBrokerAndSecretDefaults(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			s := mustLoad(t)
			if s.Kafka.Kafka.Brokers == "" {
				t.Error("KAFKA_BROKERS not filled for a local ENV")
			}
			if s.LogKeyFP.Secret == "" {
				t.Error("LOG_KEY_FP_SECRET not filled for a local ENV")
			}
		})
	}
}

// Scenario: Shutdown timing is a Go duration
func TestShutdownTimingIsGoDuration(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.ShutdownGrace != 10*time.Second {
		t.Errorf("SHUTDOWN_GRACE default = %s, want 10s", s.ShutdownGrace)
	}
	if s.DrainDelay != 0 {
		t.Errorf("SHUTDOWN_DRAIN_DELAY default = %s, want 0s", s.DrainDelay)
	}
	t.Setenv("SHUTDOWN_GRACE", "15s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "2s")
	s = mustLoad(t)
	if s.ShutdownGrace != 15*time.Second || s.DrainDelay != 2*time.Second {
		t.Errorf("grace/drain = %s/%s, want 15s/2s", s.ShutdownGrace, s.DrainDelay)
	}
	t.Setenv("SHUTDOWN_GRACE", "soon")
	_, err := config.Load()
	mustErrNaming(t, err, "SHUTDOWN_GRACE")
}

// Scenario: A non-positive batch size is rejected
func TestValidateRejectsNonPositiveBatchSize(t *testing.T) {
	for _, v := range []string{"0", "-5"} {
		t.Run(v, func(t *testing.T) {
			localEnv(t)
			t.Setenv("BATCH_MAX_SIZE", v)
			_, err := config.Load()
			mustErrNaming(t, err, "BATCH_MAX_SIZE")
		})
	}
}

// Scenario: A missing DuckDB path is rejected
func TestValidateRejectsEmptyDuckDBPath(t *testing.T) {
	localEnv(t)
	t.Setenv("WAREHOUSE_DRIVER", "duckdb")
	t.Setenv("DUCKDB_PATH", "   ")
	_, err := config.Load()
	mustErrNaming(t, err, "DUCKDB_PATH")
}

// Scenario: A missing DuckDB path is rejected (driver rules are kept)
func TestValidateRejectsUnknownDriverAndIncompleteBigQuery_AMissingDuckDBPathIsRejected(t *testing.T) {
	localEnv(t)
	t.Setenv("WAREHOUSE_DRIVER", "sqlite")
	_, err := config.Load()
	mustErrNaming(t, err, "WAREHOUSE_DRIVER")

	localEnv(t)
	t.Setenv("WAREHOUSE_DRIVER", "bigquery")
	t.Setenv("BIGQUERY_PROJECT", "   ")
	_, err = config.Load()
	mustErrNaming(t, err, "BIGQUERY_PROJECT")
}

// Scenario: Only the allowed enable flags are declared
func TestNoDependencyEnableFlags(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range declared {
		for _, gone := range legacyKeys {
			if k == gone {
				t.Errorf("removed key %s is still declared", k)
			}
		}
		if strings.HasSuffix(k, "_ENABLED") && k != "OTEL_ENABLED" && k != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("enable flag %s is declared", k)
		}
	}
}

// Scenario: A removed KAFKA_ENABLED value is ignored
func TestRemovedKafkaEnabledIsIgnored(t *testing.T) {
	localEnv(t)
	want := mustLoad(t)
	for _, v := range []string{"true", "false", "not-a-bool"} {
		t.Setenv("KAFKA_ENABLED", v)
		got, err := config.Load()
		if err != nil {
			t.Fatalf("KAFKA_ENABLED=%s: %v", v, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("KAFKA_ENABLED=%s changed the settings", v)
		}
	}
}

// Scenario: The removed GA4_EXPORT_SOURCE has no effect
func TestRemovedGA4ExportSourceIsIgnored(t *testing.T) {
	localEnv(t)
	want := mustLoad(t)
	for _, v := range []string{"fixture", "bigquery", "bogus"} {
		t.Setenv("GA4_EXPORT_SOURCE", v)
		got, err := config.Load()
		if err != nil {
			t.Fatalf("GA4_EXPORT_SOURCE=%s: %v", v, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GA4_EXPORT_SOURCE=%s changed the settings", v)
		}
	}
	t.Setenv("GA4_RELAY_PROBE_PORT", "70000")
	if _, err := config.Load(); err != nil {
		t.Errorf("removed GA4_RELAY_PROBE_PORT still validated: %v", err)
	}
}

// Scenario: Reflection setting defaults to off
func TestReflectionDefaultsToOff(t *testing.T) {
	localEnv(t)
	if s := mustLoad(t); s.ReflectionEnabled {
		t.Error("GRPC_REFLECTION_ENABLED defaults to true, want false")
	}
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	if s := mustLoad(t); !s.ReflectionEnabled {
		t.Error("GRPC_REFLECTION_ENABLED=true not honoured in a local ENV")
	}
}

// Scenario: A strict ENV refuses reflection
func TestStrictEnvRefusesReflection(t *testing.T) {
	strictEnv(t, "production")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
}

// Scenario: A strict ENV refuses mock data before other validation
func TestStrictEnvRefusesMockDataBeforeValidation(t *testing.T) {
	strictEnv(t, "production")
	t.Setenv("MOCK_DATA", "ga4export")
	for _, k := range []string{"GA4_API_SECRET", "GA4_MEASUREMENT_ID", "GA4_EXPORT_DATASET"} {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	_, err := config.Load()
	mustErrNaming(t, err, "MOCK_DATA")
	for _, k := range []string{"GA4_API_SECRET", "GA4_MEASUREMENT_ID", "GA4_EXPORT_DATASET"} {
		if strings.Contains(err.Error(), k) {
			t.Errorf("error %q names %s, the mock data refusal must come first", err.Error(), k)
		}
	}
}

// Scenario: A strict ENV requires the GA4 relay credentials
func TestStrictEnvRequiresGA4Credentials(t *testing.T) {
	for _, key := range []string{"GA4_API_SECRET", "GA4_MEASUREMENT_ID"} {
		t.Run(key, func(t *testing.T) {
			strictEnv(t, "staging")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load()
			mustErrNaming(t, err, key)
		})
	}
	strictEnv(t, "staging")
	if _, err := config.Load(); err != nil {
		t.Fatalf("strict ENV with every required key: %v", err)
	}
}

// Scenario: A strict ENV requires the GA4 export dataset
func TestStrictEnvRequiresGA4ExportDataset(t *testing.T) {
	strictEnv(t, "production")
	if err := os.Unsetenv("GA4_EXPORT_DATASET"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "GA4_EXPORT_DATASET")
}

// Scenario: A local ENV fills the defaults (GA4 stays optional locally)
func TestLocalEnvKeepsGA4Optional_ALocalENVFillsTheDefaults(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.GA4RelayEnabled() {
		t.Error("relay enabled without credentials")
	}
	if s.GA4.ExportDataset != "" {
		t.Errorf("GA4_EXPORT_DATASET default = %q, want empty", s.GA4.ExportDataset)
	}
}

// Scenario: OTEL_SERVICE_NAME is an accepted key
func TestOTelServiceNameIsAcceptedKey(t *testing.T) {
	found := false
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		if k == "OTEL_SERVICE_NAME" {
			found = true
		}
	}
	if !found {
		t.Error("OTEL_SERVICE_NAME is not declared")
	}
	localEnv(t)
	t.Setenv("OTEL_SERVICE_NAME", "analytics-custom")
	if s := mustLoad(t); s.OTel.ServiceName != "analytics-custom" {
		t.Errorf("OTel.ServiceName = %q, want analytics-custom", s.OTel.ServiceName)
	}
}

// Scenario: A local ENV fills the defaults (new topics, trigger URL, keepempty metrics address)
func TestDefaultsOfNewKeys_ALocalENVFillsTheDefaults(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.Port != 50059 {
		t.Errorf("GRPC_PORT = %d, want 50059", s.Port)
	}
	if s.Kafka.AnalyticsTopic != "analytics.events" {
		t.Errorf("KAFKA_ANALYTICS_TOPIC = %q", s.Kafka.AnalyticsTopic)
	}
	if s.Kafka.DLQTopic != "analytics.events.dlq" {
		t.Errorf("KAFKA_DLQ_TOPIC = %q, want analytics.events.dlq", s.Kafka.DLQTopic)
	}
	if s.Kafka.ListingDLQTopic != "listing.events.analytics.dlq" {
		t.Errorf("KAFKA_LISTING_DLQ_TOPIC = %q, want listing.events.analytics.dlq", s.Kafka.ListingDLQTopic)
	}
	if s.GA4.ImportTriggerURL != "http://localhost:9109/internal/ga4import" {
		t.Errorf("GA4_IMPORT_TRIGGER_URL = %q", s.GA4.ImportTriggerURL)
	}
	if s.Warehouse.Driver != config.DriverDuckDB {
		t.Errorf("WAREHOUSE_DRIVER = %q, want duckdb", s.Warehouse.Driver)
	}
	if s.Batch.MaxSize != 500 {
		t.Errorf("BATCH_MAX_SIZE = %d, want 500", s.Batch.MaxSize)
	}
	if s.Metrics.Addr != ":9109" {
		t.Errorf("METRICS_ADDR = %q, want :9109", s.Metrics.Addr)
	}
	t.Setenv("METRICS_ADDR", "")
	if s := mustLoad(t); s.Metrics.Addr != "" {
		t.Errorf("explicit empty METRICS_ADDR = %q, want empty (disables the listener)", s.Metrics.Addr)
	}
	t.Setenv("KAFKA_DLQ_TOPIC", "custom.dlq")
	if s := mustLoad(t); s.Kafka.DLQTopic != "custom.dlq" {
		t.Errorf("KAFKA_DLQ_TOPIC override = %q", s.Kafka.DLQTopic)
	}
}

func TestTrackingDefaultsAndValidation(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.Tracking.StaleAfterSeconds != 900 || s.Tracking.LagP95MaxSeconds != 300 || s.Tracking.MissingListingMaxRatio != 0.05 {
		t.Errorf("tracking defaults = %+v, want 900/300/0.05", s.Tracking)
	}
	for key, v := range map[string]string{
		"TRACKING_STALE_AFTER_SECONDS":       "0",
		"TRACKING_LAG_P95_MAX_SECONDS":       "0",
		"TRACKING_MISSING_LISTING_MAX_RATIO": "1.1",
	} {
		t.Run(key, func(t *testing.T) {
			localEnv(t)
			t.Setenv(key, v)
			_, err := config.Load()
			mustErrNaming(t, err, key)
		})
	}
	localEnv(t)
	t.Setenv("TRACKING_MISSING_LISTING_MAX_RATIO", "-0.1")
	_, err := config.Load()
	mustErrNaming(t, err, "TRACKING_MISSING_LISTING_MAX_RATIO")
}

func TestRecsAttributionDefaultAndValidation(t *testing.T) {
	localEnv(t)
	if s := mustLoad(t); s.Recs.AttributionWindowHours != 24 {
		t.Errorf("attribution window default = %d, want 24", s.Recs.AttributionWindowHours)
	}
	t.Setenv("RECS_ATTRIBUTION_WINDOW_HOURS", "0")
	_, err := config.Load()
	mustErrNaming(t, err, "RECS_ATTRIBUTION_WINDOW_HOURS")
}

// Scenario: Invalid threshold is rejected at startup
func TestInvalidGA4ReconcileThresholdIsRejectedAtStartup(t *testing.T) {
	for _, v := range []string{"-1", "abc"} {
		localEnv(t)
		t.Setenv("GA4_RECONCILE_MAX_DIFF_RATIO", v)
		_, err := config.Load()
		mustErrNaming(t, err, "GA4_RECONCILE_MAX_DIFF_RATIO")
	}
}

// Scenario: The job is a no-op when unconfigured
func TestGA4SettingsDefaults(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.GA4.ExportDataset != "" {
		t.Errorf("GA4_EXPORT_DATASET default = %q, want empty", s.GA4.ExportDataset)
	}
	if s.GA4.ReconcileMaxDiffRatio != 0.35 {
		t.Errorf("GA4_RECONCILE_MAX_DIFF_RATIO default = %v, want 0.35", s.GA4.ReconcileMaxDiffRatio)
	}
}

// Scenario: Disabled without secret
func TestGA4RelayDefaults_DisabledWithoutSecret(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
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
		t.Errorf("GA4_RELAY_DLQ_TOPIC default = %q", s.GA4.RelayDLQTopic)
	}
	if s.GA4.RelayConsumerGroup == "" ||
		s.GA4.RelayConsumerGroup == s.Kafka.ConsumerGroup ||
		s.GA4.RelayConsumerGroup == s.Engagement.ConsumerGroup ||
		s.GA4.RelayConsumerGroup == s.Kafka.ListingConsumerGroup {
		t.Errorf("relay consumer group %q must be non-empty and distinct from the other groups", s.GA4.RelayConsumerGroup)
	}
}

// Scenario: Disabled without secret
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
			localEnv(t)
			t.Setenv("GA4_API_SECRET", c.secret)
			t.Setenv("GA4_MEASUREMENT_ID", c.mid)
			s := mustLoad(t)
			if got := s.GA4RelayEnabled(); got != c.want {
				t.Errorf("GA4RelayEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}

// Scenario: No Google traffic in e2e
func TestGA4RelayEnvOverrides_DisabledWithoutSecret(t *testing.T) {
	localEnv(t)
	t.Setenv("GA4_API_SECRET", "s3cret")
	t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
	t.Setenv("GA4_ENDPOINT", "http://ga4-mock:8080/mp/collect")
	t.Setenv("GA4_RELAY_DLQ_TOPIC", "custom.dlq")
	t.Setenv("GA4_RELAY_CONSUMER_GROUP", "custom-group")
	s := mustLoad(t)
	if s.GA4.APISecret != "s3cret" || s.GA4.MeasurementID != "G-ABC123" {
		t.Errorf("secret/measurement id not loaded: %q / %q", s.GA4.APISecret, s.GA4.MeasurementID)
	}
	if s.GA4.Endpoint != "http://ga4-mock:8080/mp/collect" || s.GA4.RelayDLQTopic != "custom.dlq" ||
		s.GA4.RelayConsumerGroup != "custom-group" {
		t.Errorf("overrides not applied: %+v", s.GA4)
	}
}

// Scenario: Disabled without secret (an enabled relay needs a usable endpoint, DLQ and group)
func TestGA4RelayValidate_EnabledRequiresUsableConfig_DisabledWithoutSecret(t *testing.T) {
	for key, v := range map[string]string{
		"GA4_ENDPOINT":             " ",
		"GA4_RELAY_DLQ_TOPIC":      " ",
		"GA4_RELAY_CONSUMER_GROUP": " ",
	} {
		t.Run(key, func(t *testing.T) {
			localEnv(t)
			t.Setenv("GA4_API_SECRET", "s3cret")
			t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
			t.Setenv(key, v)
			_, err := config.Load()
			mustErrNaming(t, err, key)
		})
	}
}

// Scenario: Secret never logged (the secret must not leak through error text)
func TestGA4RelayInvalidConfigErrorDoesNotContainSecret(t *testing.T) {
	localEnv(t)
	t.Setenv("GA4_API_SECRET", "TOP-SECRET-VALUE")
	t.Setenv("GA4_MEASUREMENT_ID", "G-ABC123")
	t.Setenv("GA4_ENDPOINT", " ")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "TOP-SECRET-VALUE") {
		t.Errorf("error leaks the API secret: %v", err)
	}
}
