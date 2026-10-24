package config_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-domain/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET",
	"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK", "UPSTREAM_AUDIT_ADDR", "UPSTREAM_AI_ADDR",
	"UPSTREAM_AI_TIMEOUT_SECONDS", "KAFKA_LISTING_TOPIC", "KAFKA_IDENTITY_TOPIC", "KAFKA_IDENTITY_GROUP",
	"STORAGE_ENDPOINT", "STORAGE_BUCKET", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY", "STORAGE_REGION",
	"STORAGE_USE_SSL", "STORAGE_PUBLIC_BASE_URL", "RESERVATION_TTL", "RESERVATION_SWEEP_INTERVAL",
	"SCREEN_TERMS_FILE", "DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "IDENTITY_CONSUMER_ENABLED",
	"OUTBOX_MAX_ATTEMPTS", "OUTBOX_CLAIM_LOCK_SECONDS", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS",
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
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	t.Setenv("UPSTREAM_AI_ADDR", "team-ai:50051")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
	t.Setenv("STORAGE_ACCESS_KEY", "ak")
	t.Setenv("STORAGE_SECRET_KEY", "sk")
}

func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func mustContain(t *testing.T, err error, names ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %v, got nil", names)
	}
	for _, n := range names {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q must name %s", err, n)
		}
	}
}

func valueByEnv(t *testing.T, s any, key string) any {
	t.Helper()
	var find func(reflect.Value) (any, bool)
	find = func(v reflect.Value) (any, bool) {
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if tag, ok := f.Tag.Lookup("env"); ok {
				if k, _, _ := strings.Cut(tag, ","); k == key {
					return v.Field(i).Interface(), true
				}
				continue
			}
			if v.Field(i).Kind() == reflect.Struct {
				if got, ok := find(v.Field(i)); ok {
					return got, true
				}
			}
		}
		return nil, false
	}
	got, ok := find(reflect.ValueOf(s))
	if !ok {
		t.Fatalf("no Settings field bound to %s", key)
	}
	return got
}

func TestLoadRequiresEnv(t *testing.T) {
	localEnv(t)
	unset(t, "ENV")
	_, err := config.Load()
	mustContain(t, err, "ENV")
}

func TestLoadRequiresGRPCPort(t *testing.T) {
	localEnv(t)
	unset(t, "GRPC_PORT")
	_, err := config.Load()
	mustContain(t, err, "GRPC_PORT")
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	localEnv(t)
	unset(t, "DATABASE_URL")
	_, err := config.Load()
	mustContain(t, err, "DATABASE_URL")
	t.Setenv("DATABASE_URL", "")
	_, err = config.Load()
	mustContain(t, err, "DATABASE_URL")
}

func TestLoadDefaultsFilledLocally(t *testing.T) {
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := valueByEnv(t, s, "GRPC_PORT"); got != 50051 {
		t.Errorf("GRPC_PORT = %v, want 50051", got)
	}
	if got := valueByEnv(t, s, "KAFKA_LISTING_TOPIC"); got != "listing.events" {
		t.Errorf("KAFKA_LISTING_TOPIC = %v", got)
	}
	if got := valueByEnv(t, s, "KAFKA_IDENTITY_TOPIC"); got != "identity.events" {
		t.Errorf("KAFKA_IDENTITY_TOPIC = %v", got)
	}
	if got := valueByEnv(t, s, "KAFKA_IDENTITY_GROUP"); got != "team-domain-identity" {
		t.Errorf("KAFKA_IDENTITY_GROUP = %v", got)
	}
	if got := valueByEnv(t, s, "UPSTREAM_AI_TIMEOUT_SECONDS"); got != float64(2) {
		t.Errorf("UPSTREAM_AI_TIMEOUT_SECONDS = %v, want 2", got)
	}
}

func TestLocalEnvAcceptsDefaultsAndReflection(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		localEnv(t)
		t.Setenv("ENV", env)
		s, err := config.Load()
		if err != nil {
			t.Fatalf("ENV=%s Load: %v", env, err)
		}
		if got := valueByEnv(t, s, "KAFKA_BROKERS"); got != "localhost:9092" {
			t.Errorf("ENV=%s KAFKA_BROKERS = %v, want localhost:9092", env, got)
		}
		if got := valueByEnv(t, s, "GRPC_REFLECTION_ENABLED"); got != false {
			t.Errorf("ENV=%s reflection default = %v, want false", env, got)
		}
		if got := valueByEnv(t, s, "LOG_KEY_FP_SECRET"); got == "" {
			t.Errorf("ENV=%s LOG_KEY_FP_SECRET must be filled with the local default", env)
		}
		t.Setenv("GRPC_REFLECTION_ENABLED", "true")
		s, err = config.Load()
		if err != nil {
			t.Fatalf("ENV=%s Load with reflection: %v", env, err)
		}
		if got := valueByEnv(t, s, "GRPC_REFLECTION_ENABLED"); got != true {
			t.Errorf("ENV=%s reflection = %v, want true", env, got)
		}
	}
}

func TestStrictEnvRefusesUnsafeSettings(t *testing.T) {
	envs := []string{"staging", "stage", "prod", "production", " Production "}
	cases := []struct {
		name   string
		mutate func(*testing.T)
		want   string
	}{
		{"reflection", func(t *testing.T) { t.Setenv("GRPC_REFLECTION_ENABLED", "true") }, "GRPC_REFLECTION_ENABLED"},
		{"kafka brokers empty", func(t *testing.T) { t.Setenv("KAFKA_BROKERS", "") }, "KAFKA_BROKERS"},
		{"log key secret empty", func(t *testing.T) { t.Setenv("LOG_KEY_FP_SECRET", "") }, "LOG_KEY_FP_SECRET"},
		{"audit addr empty", func(t *testing.T) { t.Setenv("UPSTREAM_AUDIT_ADDR", "") }, "UPSTREAM_AUDIT_ADDR"},
		{"ai addr empty", func(t *testing.T) { t.Setenv("UPSTREAM_AI_ADDR", "") }, "UPSTREAM_AI_ADDR"},
		{"storage access key default", func(t *testing.T) { t.Setenv("STORAGE_ACCESS_KEY", "minioadmin") }, "STORAGE_ACCESS_KEY"},
		{"storage secret key default", func(t *testing.T) { t.Setenv("STORAGE_SECRET_KEY", "minioadmin") }, "STORAGE_SECRET_KEY"},
		{"database url empty", func(t *testing.T) { t.Setenv("DATABASE_URL", "") }, "DATABASE_URL"},
	}
	for _, env := range envs {
		strictEnv(t, env)
		if _, err := config.Load(); err != nil {
			t.Fatalf("ENV=%q safe config must load: %v", env, err)
		}
		for _, c := range cases {
			strictEnv(t, env)
			c.mutate(t)
			_, err := config.Load()
			if err == nil {
				t.Errorf("ENV=%q %s: must be refused", env, c.name)
				continue
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("ENV=%q %s: error %q must name %s", env, c.name, err, c.want)
			}
		}
	}
}

func TestStrictEnvReportsSDKRefusalsWithoutDatabaseURL(t *testing.T) {
	strictEnv(t, "production")
	unset(t, "DATABASE_URL", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	mustContain(t, err, "DATABASE_URL", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET", "GRPC_REFLECTION_ENABLED")
}

func TestUpstreamAddressesRequiredInEveryEnv(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		for _, key := range []string{"UPSTREAM_AI_ADDR", "UPSTREAM_AUDIT_ADDR"} {
			localEnv(t)
			t.Setenv("ENV", env)
			unset(t, key)
			_, err := config.Load()
			mustContain(t, err, key)

			t.Setenv(key, "")
			_, err = config.Load()
			mustContain(t, err, key)
		}
	}
}

func TestUpstreamAuditConfig(t *testing.T) {
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := valueByEnv(t, s, "UPSTREAM_AUDIT_ADDR"); got != "team-audit-svc:50066" {
		t.Errorf("UPSTREAM_AUDIT_ADDR = %v, want team-audit-svc:50066", got)
	}
	unset(t, "UPSTREAM_AUDIT_ADDR")
	_, err = config.Load()
	mustContain(t, err, "UPSTREAM_AUDIT_ADDR")

	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if !contains(keys, "UPSTREAM_AUDIT_ADDR") {
		t.Error("Settings does not declare UPSTREAM_AUDIT_ADDR")
	}
	if contains(keys, "UPSTREAM_CALL_TIMEOUT_SECONDS") {
		t.Error("Settings must not declare UPSTREAM_CALL_TIMEOUT_SECONDS")
	}
	if err := sdkconfig.CheckEnvExample(filepath.Join("..", "..", ".env.example"), config.Settings{}); err != nil {
		t.Errorf("env drift gate: %v", err)
	}
}

func contains(keys []string, k string) bool {
	for _, x := range keys {
		if x == k {
			return true
		}
	}
	return false
}

func TestNoRemovedEnvKeysDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	removed := []string{
		"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "IDENTITY_CONSUMER_ENABLED",
		"OUTBOX_MAX_ATTEMPTS", "OUTBOX_CLAIM_LOCK_SECONDS", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS",
	}
	for _, k := range removed {
		if contains(keys, k) {
			t.Errorf("%s must not be declared", k)
		}
	}
	var enabled []string
	for _, k := range keys {
		if strings.HasSuffix(k, "_ENABLED") {
			enabled = append(enabled, k)
		}
	}
	want := []string{"GRPC_REFLECTION_ENABLED", "OTEL_ENABLED"}
	if len(enabled) != len(want) || !contains(enabled, want[0]) || !contains(enabled, want[1]) {
		t.Errorf("declared _ENABLED names = %v, want exactly %v", enabled, want)
	}
	for _, k := range []string{"SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "OUTBOX_CLAIM_LOCK", "LOG_KEY_FP_SECRET", "UPSTREAM_AI_ADDR", "UPSTREAM_AUDIT_ADDR"} {
		if !contains(keys, k) {
			t.Errorf("%s must be declared", k)
		}
	}
}

func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if !contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "LOG_KEY_FP_SECRET") {
		t.Fatal("Settings must declare the SDK groups (LOG_KEY_FP_SECRET missing)")
	}
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestEnvExampleDocumentsValues(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	has := func(line string) bool {
		for _, l := range lines {
			if strings.TrimSpace(l) == line {
				return true
			}
		}
		return false
	}
	for _, line := range []string{"GRPC_PORT=50051", "GRPC_REFLECTION_ENABLED=false"} {
		if !has(line) {
			t.Errorf(".env.example must list %s", line)
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "ENV=") && strings.Contains(l, "default") {
			t.Errorf("ENV line must not claim a default: %q", l)
		}
	}
}

func TestScreenTermsFileMissingFails(t *testing.T) {
	localEnv(t)
	missing := t.TempDir() + "/does-not-exist.txt"
	t.Setenv("SCREEN_TERMS_FILE", missing)
	_, err := config.Load()
	mustContain(t, err, "SCREEN_TERMS_FILE")

	p := t.TempDir() + "/terms.txt"
	if err := os.WriteFile(p, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCREEN_TERMS_FILE", p)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("existing terms file: %v", err)
	}
	if got := valueByEnv(t, s, "SCREEN_TERMS_FILE"); got != p {
		t.Errorf("SCREEN_TERMS_FILE = %v, want %v", got, p)
	}
	unset(t, "SCREEN_TERMS_FILE")
	if _, err := config.Load(); err != nil {
		t.Fatalf("unset terms file: %v", err)
	}
}

func TestLoadRejectsBadPortAndNegativeDurations(t *testing.T) {
	cases := []struct{ key, val string }{
		{"GRPC_PORT", "0"},
		{"GRPC_PORT", "-1"},
		{"SHUTDOWN_GRACE", "-1s"},
		{"SHUTDOWN_DRAIN_DELAY", "-1s"},
	}
	for _, c := range cases {
		localEnv(t)
		t.Setenv(c.key, c.val)
		_, err := config.Load()
		if err == nil {
			t.Errorf("%s=%s must be refused", c.key, c.val)
			continue
		}
		if !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%s: error %q must name the variable", c.key, c.val, err)
		}
	}
	localEnv(t)
	t.Setenv("SHUTDOWN_GRACE", "0s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "0s")
	if _, err := config.Load(); err != nil {
		t.Errorf("zero durations must load: %v", err)
	}
}

func reservationSettings(t *testing.T, env map[string]string) config.Settings {
	t.Helper()
	localEnv(t)
	for k, v := range env {
		t.Setenv(k, v)
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestReservationDefaults(t *testing.T) {
	s := reservationSettings(t, nil)
	logger, buf := captureLogger()
	if got := s.ReservationTTL(logger); got != 15*time.Minute {
		t.Errorf("default RESERVATION_TTL = %v, want 15m", got)
	}
	if got := s.ReservationSweepInterval(logger); got != time.Minute {
		t.Errorf("default RESERVATION_SWEEP_INTERVAL = %v, want 1m", got)
	}
	if buf.Len() != 0 {
		t.Errorf("defaults must not warn, got %q", buf.String())
	}
}

func TestReservationOverride(t *testing.T) {
	s := reservationSettings(t, map[string]string{"RESERVATION_TTL": "20s", "RESERVATION_SWEEP_INTERVAL": "2s"})
	logger, buf := captureLogger()
	if got := s.ReservationTTL(logger); got != 20*time.Second {
		t.Errorf("RESERVATION_TTL = %v, want 20s", got)
	}
	if got := s.ReservationSweepInterval(logger); got != 2*time.Second {
		t.Errorf("RESERVATION_SWEEP_INTERVAL = %v, want 2s", got)
	}
	if buf.Len() != 0 {
		t.Errorf("valid overrides must not warn, got %q", buf.String())
	}
}

func TestReservationInvalidFallsBackWithWarning(t *testing.T) {
	for _, bad := range []string{"banana", "0", "0s", "-5m", ""} {
		s := reservationSettings(t, map[string]string{"RESERVATION_TTL": bad, "RESERVATION_SWEEP_INTERVAL": bad})
		logger, buf := captureLogger()
		if got := s.ReservationTTL(logger); got != 15*time.Minute {
			t.Errorf("TTL %q -> %v, want default 15m", bad, got)
		}
		if got := s.ReservationSweepInterval(logger); got != time.Minute {
			t.Errorf("interval %q -> %v, want default 1m", bad, got)
		}
		out := buf.String()
		for _, want := range []string{"level=WARN", "RESERVATION_TTL", "RESERVATION_SWEEP_INTERVAL", "default=15m0s", "default=1m0s"} {
			if !strings.Contains(out, want) {
				t.Errorf("value %q: warning log %q lacks %q", bad, out, want)
			}
		}
	}
}

func TestAIUpstreamFromEnv(t *testing.T) {
	localEnv(t)
	t.Setenv("UPSTREAM_AI_ADDR", "team-ai:50051")
	t.Setenv("UPSTREAM_AI_TIMEOUT_SECONDS", "1.5")
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := valueByEnv(t, s, "UPSTREAM_AI_ADDR"); got != "team-ai:50051" {
		t.Errorf("UPSTREAM_AI_ADDR = %v", got)
	}
	if got := valueByEnv(t, s, "UPSTREAM_AI_TIMEOUT_SECONDS"); got != 1.5 {
		t.Errorf("UPSTREAM_AI_TIMEOUT_SECONDS = %v, want 1.5", got)
	}
}
