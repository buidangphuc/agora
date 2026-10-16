package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Server.Port != 50054 {
		t.Errorf("default GRPC_PORT = %d, want 50054", s.Server.Port)
	}
}

func TestKafkaDefaultsAndValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Kafka.Enabled || s.Kafka.ListingTopic != "listing.events" || s.Kafka.ConsumerGroup != "team-engagement-feed" {
		t.Errorf("unexpected kafka defaults: %+v", s.Kafka)
	}
	t.Setenv("KAFKA_ENABLED", "true")
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092")
	s, err = LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings enabled: %v", err)
	}
	if got := s.KafkaBrokers(); len(got) != 2 || got[1] != "b:9092" {
		t.Errorf("brokers = %v", got)
	}
	t.Setenv("KAFKA_BROKERS", " ")
	if _, err := LoadSettings(); err == nil {
		t.Error("enabled with no brokers must fail validation")
	}
}

func TestEngagementEventsDefaultsAndValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Kafka.EventsTopic != "engagement.events" || s.Kafka.OutboxRelayInterval != 500*time.Millisecond {
		t.Errorf("defaults: %+v", s.Kafka)
	}
	t.Setenv("ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "2s")
	if s, err = LoadSettings(); err != nil || s.Kafka.OutboxRelayInterval != 2*time.Second {
		t.Errorf("interval: %v %v", s, err)
	}
	t.Setenv("ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "0s")
	if _, err := LoadSettings(); err == nil {
		t.Error("zero interval must fail")
	}
}

// Scenario: No audit upstream configured / config for UPSTREAM_AUDIT_ADDR.
func TestUpstreamAuditAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.AuditAddr != "" {
		t.Errorf("default UPSTREAM_AUDIT_ADDR = %q, want empty (audit disabled)", s.Upstream.AuditAddr)
	}
	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	s, err = LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.AuditAddr != "team-audit-svc:50066" {
		t.Errorf("AuditAddr = %q", s.Upstream.AuditAddr)
	}
}

// settingByEnvKey returns the value of the Settings field tagged env:"key",
// found by reflection so the test does not depend on the group/field names.
func settingByEnvKey(t *testing.T, s *Settings, key string) any {
	t.Helper()
	v := reflect.ValueOf(s).Elem()
	for i := 0; i < v.NumField(); i++ {
		g := v.Field(i)
		for j := 0; j < g.NumField(); j++ {
			if g.Type().Field(j).Tag.Get("env") == key {
				return g.Field(j).Interface()
			}
		}
	}
	t.Fatalf("no setting declared for %s", key)
	return nil
}

func toInt64(t *testing.T, v any) int64 {
	t.Helper()
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case time.Duration:
		return int64(x)
	}
	t.Fatalf("unexpected type %T", v)
	return 0
}

// Scenario: RT Listing reports: Retention config is validated (defaults).
func TestReportRetentionConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got := toInt64(t, settingByEnvKey(t, s, "REPORT_RETENTION_DAYS")); got != 730 {
		t.Errorf("REPORT_RETENTION_DAYS default = %d, want 730", got)
	}
	if got := time.Duration(toInt64(t, settingByEnvKey(t, s, "REPORT_SWEEP_INTERVAL"))); got != 24*time.Hour {
		t.Errorf("REPORT_SWEEP_INTERVAL default = %v, want 24h", got)
	}
	t.Setenv("REPORT_RETENTION_DAYS", "30")
	t.Setenv("REPORT_SWEEP_INTERVAL", "1h")
	s, err = LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings overrides: %v", err)
	}
	if got := toInt64(t, settingByEnvKey(t, s, "REPORT_RETENTION_DAYS")); got != 30 {
		t.Errorf("REPORT_RETENTION_DAYS = %d, want 30", got)
	}
	if got := time.Duration(toInt64(t, settingByEnvKey(t, s, "REPORT_SWEEP_INTERVAL"))); got != time.Hour {
		t.Errorf("REPORT_SWEEP_INTERVAL = %v, want 1h", got)
	}
}

// Scenario: RT Listing reports: Retention config is validated.
func TestReportRetentionConfigValidated(t *testing.T) {
	cases := []struct{ name, key, val string }{
		{"retention zero", "REPORT_RETENTION_DAYS", "0"},
		{"retention negative", "REPORT_RETENTION_DAYS", "-5"},
		{"sweep interval zero", "REPORT_SWEEP_INTERVAL", "0s"},
		{"sweep interval negative", "REPORT_SWEEP_INTERVAL", "-1h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://x/y")
			t.Setenv(c.key, c.val)
			_, err := LoadSettings()
			if err == nil {
				t.Fatalf("%s=%s must fail validation", c.key, c.val)
			}
			if !strings.Contains(err.Error(), c.key) {
				t.Errorf("error %q must name %s", err, c.key)
			}
		})
	}
}
