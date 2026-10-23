package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-engagement/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "DATABASE_ENABLED", "KAFKA_ENABLED",
	"KAFKA_BROKERS", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_ORDER_ADDR",
	"UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_SERVICE_NAME", "KAFKA_CONSUMER_GROUP", "KAFKA_LISTING_TOPIC", "ENGAGEMENT_EVENTS_TOPIC",
	"ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
	"REPORT_RETENTION_DAYS", "REPORT_SWEEP_INTERVAL",
}

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
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
	t.Setenv("GRPC_PORT", "50054")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_ORDER_ADDR", "order:50055")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "audit:50066")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
}

func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func mustErrNaming(t *testing.T, err error, keys ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %v, got nil", keys)
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err, k)
		}
	}
}

func settingByEnvKey(t *testing.T, s any, key string) any {
	t.Helper()
	var walk func(v reflect.Value) (any, bool)
	walk = func(v reflect.Value) (any, bool) {
		v = reflect.Indirect(v)
		if v.Kind() != reflect.Struct {
			return nil, false
		}
		for i := 0; i < v.NumField(); i++ {
			ft := v.Type().Field(i)
			if !ft.IsExported() {
				continue
			}
			if tag, ok := ft.Tag.Lookup("env"); ok && strings.Split(tag, ",")[0] == key {
				return v.Field(i).Interface(), true
			}
			if got, ok := walk(v.Field(i)); ok {
				return got, true
			}
		}
		return nil, false
	}
	got, ok := walk(reflect.ValueOf(s))
	if !ok {
		t.Fatalf("no setting declared for %s", key)
	}
	return got
}

func asInt64(t *testing.T, v any) int64 {
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

func TestLoadRefusesMissingEnv(t *testing.T) {
	// Scenario: Engagement SDK: Missing ENV is refused
	localEnv(t)
	unset(t, "ENV")
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")
}

func TestLoadRefusesEmptyDatabaseURL(t *testing.T) {
	// Scenario: Engagement SDK: Empty database url is refused in every ENV
	for _, env := range []string{"local", "test", "dev"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("DATABASE_URL", "")
			_, err := config.Load()
			mustErrNaming(t, err, "DATABASE_URL")
		})
	}
}

func TestLoadRefusesEmptyOrderAddr(t *testing.T) {
	// Scenario: Engagement SDK: Empty order address is refused in every ENV
	// Scenario: No order upstream configured (review-integrity)
	for _, env := range []string{"local", "test", "dev"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("UPSTREAM_ORDER_ADDR", "")
			_, err := config.Load()
			mustErrNaming(t, err, "UPSTREAM_ORDER_ADDR")
		})
	}
	localEnv(t)
	unset(t, "UPSTREAM_ORDER_ADDR")
	_, err := config.Load()
	mustErrNaming(t, err, "UPSTREAM_ORDER_ADDR")
}

func TestLoadRefusesEmptyAuditAddr(t *testing.T) {
	// Scenario: Engagement SDK: Empty audit address is refused in every ENV
	// Scenario: No audit upstream configured (review-moderation)
	// Scenario: Dispute audit: No audit upstream configured
	// Scenario: Listing reports: No audit upstream configured
	for _, env := range []string{"local", "test", "dev"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("UPSTREAM_AUDIT_ADDR", "")
			_, err := config.Load()
			mustErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")
		})
	}
	localEnv(t)
	unset(t, "UPSTREAM_AUDIT_ADDR")
	_, err := config.Load()
	mustErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")
}

func TestMigrateSettingsNeedOnlyDatabase(t *testing.T) {
	// Scenario: Engagement SDK: The migrate job needs only the database url
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.LoadMigrate()
	if err != nil {
		t.Fatalf("LoadMigrate with only ENV and DATABASE_URL: %v", err)
	}
	if got := settingByEnvKey(t, s, "DATABASE_URL"); got != "postgres://x/y" {
		t.Errorf("DATABASE_URL = %v", got)
	}
	declared := sdkconfig.DeclaredEnvKeys(s)
	for _, k := range []string{"UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR", "KAFKA_BROKERS", "GRPC_PORT"} {
		if slices.Contains(declared, k) {
			t.Errorf("MigrateSettings must not declare %s", k)
		}
	}

	t.Setenv("DATABASE_URL", "")
	_, err = config.LoadMigrate()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestRemovedSwitchesAreNotDeclared(t *testing.T) {
	// Scenario: Engagement SDK: Removed switches are not declared and have no effect
	removed := []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS"}
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range removed {
		if slices.Contains(declared, k) {
			t.Errorf("Settings declares removed key %s", k)
		}
	}
	for _, k := range []string{"UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR", "GRPC_PORT", "DATABASE_URL", "LOG_KEY_FP_SECRET", "KAFKA_BROKERS", "OUTBOX_POLL_INTERVAL", "GRPC_REFLECTION_ENABLED"} {
		if !slices.Contains(declared, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}

	localEnv(t)
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("KAFKA_ENABLED", "false")
	t.Setenv("ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "1h")
	t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", "30")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := time.Duration(asInt64(t, settingByEnvKey(t, s, "OUTBOX_POLL_INTERVAL"))); got != time.Second {
		t.Errorf("outbox poll interval = %v, want 1s", got)
	}
	t.Setenv("DATABASE_URL", "")
	_, err = config.Load()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestLoadRefusesMissingGRPCPort(t *testing.T) {
	// Scenario: Engagement SDK: Missing port is refused
	localEnv(t)
	unset(t, "GRPC_PORT")
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_PORT")

	t.Setenv("GRPC_PORT", "")
	_, err = config.Load()
	mustErrNaming(t, err, "GRPC_PORT")
}

func TestStrictEnvRefusesReflection(t *testing.T) {
	// Scenario: Engagement SDK: Strict ENV refuses reflection
	for _, env := range []string{"staging", "prod"} {
		t.Run(env, func(t *testing.T) {
			strictEnv(t, env)
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			_, err := config.Load()
			mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
		})
	}
}

func TestUnrecognisedEnvRefusesReflectionButIsNotStrict(t *testing.T) {
	// Scenario: Engagement SDK: Unrecognised ENV refuses reflection but is not strict
	localEnv(t)
	t.Setenv("ENV", "dev")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")

	t.Setenv("GRPC_REFLECTION_ENABLED", "false")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("LOG_KEY_FP_SECRET", "")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("ENV=dev without brokers and secret must load: %v", err)
	}
	if got := settingByEnvKey(t, s, "KAFKA_BROKERS"); got != "localhost:9092" {
		t.Errorf("KAFKA_BROKERS = %v, want the SDK local default localhost:9092", got)
	}
}

func TestLocalAndTestAllowReflection(t *testing.T) {
	// Scenario: Engagement SDK: Local and test ENV allow reflection
	for _, env := range []string{"local", "test"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			s, err := config.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := settingByEnvKey(t, s, "GRPC_REFLECTION_ENABLED"); got != true {
				t.Errorf("reflection = %v, want true", got)
			}
		})
	}
}

func TestStrictEnvRequiresBrokersAuditAndSecret(t *testing.T) {
	// Scenario: Engagement SDK: Strict ENV requires brokers, audit address and fingerprint secret
	// Scenario: Engagement key redaction: strict ENV needs the secret
	localEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "")
	t.Setenv("LOG_KEY_FP_SECRET", "")
	_, err := config.Load()
	mustErrNaming(t, err, "KAFKA_BROKERS", "UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET")

	strictEnv(t, "production")
	unset(t, "LOG_KEY_FP_SECRET")
	_, err = config.Load()
	mustErrNaming(t, err, "LOG_KEY_FP_SECRET")

	strictEnv(t, "staging")
	if _, err := config.Load(); err != nil {
		t.Fatalf("fully configured strict ENV must load: %v", err)
	}
}

func TestLoadLocalDefaults(t *testing.T) {
	// Scenario: Engagement SDK: Local defaults
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]any{
		"GRPC_PORT":               50054,
		"GRPC_REFLECTION_ENABLED": false,
		"KAFKA_LISTING_TOPIC":     "listing.events",
		"KAFKA_CONSUMER_GROUP":    "team-engagement-feed",
		"ENGAGEMENT_EVENTS_TOPIC": "engagement.events",
	}
	for k, w := range want {
		if got := settingByEnvKey(t, s, k); got != w {
			t.Errorf("%s = %v, want %v", k, got, w)
		}
	}
	if got := asInt64(t, settingByEnvKey(t, s, "REPORT_RETENTION_DAYS")); got != 730 {
		t.Errorf("REPORT_RETENTION_DAYS = %d, want 730", got)
	}
	if got := time.Duration(asInt64(t, settingByEnvKey(t, s, "REPORT_SWEEP_INTERVAL"))); got != 24*time.Hour {
		t.Errorf("REPORT_SWEEP_INTERVAL = %v, want 24h", got)
	}
}

func TestEventsRequireTopicsAndGroup(t *testing.T) {
	// Scenario: Engagement SDK: Local defaults (Events.Validate requires topics and group)
	for _, k := range []string{"KAFKA_LISTING_TOPIC", "KAFKA_CONSUMER_GROUP", "ENGAGEMENT_EVENTS_TOPIC"} {
		t.Run(k, func(t *testing.T) {
			localEnv(t)
			t.Setenv(k, " ")
			_, err := config.Load()
			mustErrNaming(t, err, k)
		})
	}
}

func TestReportRetentionConfigValidated(t *testing.T) {
	// Scenario: Listing reports: Retention config is validated
	cases := []struct{ name, key, val string }{
		{"retention zero", "REPORT_RETENTION_DAYS", "0"},
		{"retention negative", "REPORT_RETENTION_DAYS", "-5"},
		{"sweep interval zero", "REPORT_SWEEP_INTERVAL", "0s"},
		{"sweep interval negative", "REPORT_SWEEP_INTERVAL", "-1h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			localEnv(t)
			t.Setenv(c.key, c.val)
			_, err := config.Load()
			mustErrNaming(t, err, c.key)
		})
	}

	localEnv(t)
	t.Setenv("REPORT_RETENTION_DAYS", "30")
	t.Setenv("REPORT_SWEEP_INTERVAL", "1h")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("valid overrides: %v", err)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "REPORT_RETENTION_DAYS")); got != 30 {
		t.Errorf("REPORT_RETENTION_DAYS = %d, want 30", got)
	}
	if got := time.Duration(asInt64(t, settingByEnvKey(t, s, "REPORT_SWEEP_INTERVAL"))); got != time.Hour {
		t.Errorf("REPORT_SWEEP_INTERVAL = %v, want 1h", got)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	// Scenario: Engagement SDK: Example file matches the declared variables
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "UPSTREAM_AUDIT_ADDR") {
		t.Error("UPSTREAM_AUDIT_ADDR must be declared")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"GRPC_PORT=50054", "ENV=local"} {
		if !strings.Contains(text, want) {
			t.Errorf(".env.example lacks %q", want)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "UPSTREAM_AUDIT_ADDR=") && strings.TrimSpace(strings.TrimPrefix(line, "UPSTREAM_AUDIT_ADDR=")) == "" {
			t.Error("UPSTREAM_AUDIT_ADDR must be listed with a value")
		}
		if strings.HasPrefix(line, "UPSTREAM_ORDER_ADDR=") && strings.TrimSpace(strings.TrimPrefix(line, "UPSTREAM_ORDER_ADDR=")) == "" {
			t.Error("UPSTREAM_ORDER_ADDR must be listed with a value")
		}
		if strings.HasPrefix(line, "GRPC_REFLECTION_ENABLED=true") {
			t.Error(".env.example must not enable reflection")
		}
	}
}
