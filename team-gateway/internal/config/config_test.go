package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-gateway/internal/config"
)

var upstreamKeys = []string{
	"UPSTREAM_SEARCH_ADDR", "UPSTREAM_LISTING_ADDR", "UPSTREAM_IDENTITY_ADDR", "UPSTREAM_ENGAGEMENT_ADDR",
	"UPSTREAM_ORDER_ADDR", "UPSTREAM_PAYMENT_ADDR", "UPSTREAM_CHAT_ADDR", "UPSTREAM_AI_ADDR",
	"UPSTREAM_RECOMMENDATION_ADDR", "UPSTREAM_PROMOTION_ADDR", "UPSTREAM_NOTIFICATION_ADDR",
	"UPSTREAM_ANALYTICS_ADDR", "UPSTREAM_REFERRAL_ADDR", "UPSTREAM_VERIFICATION_ADDR",
	"UPSTREAM_SHARING_ADDR", "UPSTREAM_AUDIT_ADDR",
}

var managedKeys = append([]string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "KAFKA_BROKERS", "KAFKA_ANALYTICS_TOPIC", "IDENTITY_EVENTS_TOPIC",
	"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "MOCK_DATA",
	"JWKS_URL", "JWKS_CACHE_TTL", "PUBLIC_SCOPES", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST",
	"CALL_TIMEOUT_SECONDS", "RETRY_MAX", "CORS_ORIGINS", "TRACK_RATE_LIMIT_RPS", "TRACK_RATE_LIMIT_BURST",
	"AI_CALL_TIMEOUT_SECONDS", "STREAM_MAX_REQUEST_BYTES", "STREAM_REVOCATION_CHECK_SECONDS",
	"PROMETHEUS_URL", "JAEGER_QUERY_URL", "JAEGER_UI_URL", "TRUSTED_PROXIES", "DIAL_TIMEOUT_SECONDS",
	"TRACK_PUBLISH_TIMEOUT",
	"HTTP_HOST", "HTTP_PORT", "SHUTDOWN_GRACE_SECONDS", "EDGE_REFLECTION_ENABLED", "KAFKA_ENABLED",
}, upstreamKeys...)

const jwksURL = "http://identity.test/.well-known/jwks.json"

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func baseEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "8080")
	t.Setenv("JWKS_URL", jwksURL)
}

func unset(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func mustErrNaming(t *testing.T, err error, keys ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Load succeeded, want an error naming %v", keys)
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err.Error(), k)
		}
	}
}

func lookup(t *testing.T, s any, key string) reflect.Value {
	t.Helper()
	var found reflect.Value
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		if v.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if tag := strings.Split(f.Tag.Get("env"), ",")[0]; tag == key {
				found = v.Field(i)
				return
			}
			walk(v.Field(i))
		}
	}
	walk(reflect.Indirect(reflect.ValueOf(s)))
	if !found.IsValid() {
		t.Fatalf("Settings declares no field for %s", key)
	}
	return found
}

func number(t *testing.T, s any, key string) float64 {
	t.Helper()
	v := lookup(t, s, key)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	}
	t.Fatalf("%s has kind %s, want a number", key, v.Kind())
	return 0
}

func text(t *testing.T, s any, key string) string {
	t.Helper()
	return lookup(t, s, key).String()
}

func duration(t *testing.T, s any, key string) time.Duration {
	t.Helper()
	v := lookup(t, s, key)
	d, ok := v.Interface().(time.Duration)
	if !ok {
		t.Fatalf("%s has type %s, want time.Duration", key, v.Type())
	}
	return d
}

func flag(t *testing.T, s any, key string) bool {
	t.Helper()
	return lookup(t, s, key).Bool()
}

func TestNoDependencyEnableFlags(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	declared := map[string]bool{}
	for _, k := range keys {
		declared[k] = true
		if strings.HasSuffix(k, "_ENABLED") && k != "OTEL_ENABLED" && k != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("enable flag %s is declared", k)
		}
	}
	for _, k := range []string{"KAFKA_ENABLED", "EDGE_REFLECTION_ENABLED", "HTTP_PORT", "SHUTDOWN_GRACE_SECONDS"} {
		if declared[k] {
			t.Errorf("retired key %s is still declared", k)
		}
	}
	for _, k := range []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "GRPC_PORT", "TRACK_PUBLISH_TIMEOUT", "MOCK_DATA"} {
		if !declared[k] {
			t.Errorf("key %s is not declared", k)
		}
	}
}

func TestLoadRequiresEnv(t *testing.T) {
	baseEnv(t)
	unset(t, "ENV")
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")
}

func TestLoadRequiresListenPort(t *testing.T) {
	baseEnv(t)
	unset(t, "GRPC_PORT")
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_PORT")
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	baseEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for key, want := range map[string]float64{
		"RATE_LIMIT_RPS": 20, "RATE_LIMIT_BURST": 40, "CALL_TIMEOUT_SECONDS": 5, "RETRY_MAX": 2,
		"JWKS_CACHE_TTL": 300, "GRPC_PORT": 8080,
	} {
		if got := number(t, s, key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got := duration(t, s, "TRACK_PUBLISH_TIMEOUT"); got != 2*time.Second {
		t.Errorf("TRACK_PUBLISH_TIMEOUT = %v, want 2s", got)
	}
	if got := duration(t, s, "SHUTDOWN_GRACE"); got != 10*time.Second {
		t.Errorf("SHUTDOWN_GRACE = %v, want 10s", got)
	}
	if got := text(t, s, "KAFKA_BROKERS"); got != "localhost:9092" {
		t.Errorf("KAFKA_BROKERS = %q, want localhost:9092", got)
	}
	if flag(t, s, "GRPC_REFLECTION_ENABLED") {
		t.Error("reflection is on by default, want off")
	}

	t.Setenv("RATE_LIMIT_RPS", "1000")
	t.Setenv("SHUTDOWN_GRACE", "3s")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("Load with overrides: %v", err)
	}
	if got := number(t, s, "RATE_LIMIT_RPS"); got != 1000 {
		t.Errorf("RATE_LIMIT_RPS = %v, want 1000", got)
	}
	if got := duration(t, s, "SHUTDOWN_GRACE"); got != 3*time.Second {
		t.Errorf("SHUTDOWN_GRACE = %v, want 3s", got)
	}
}

func TestLoadRequiresJWKSURL(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWKS_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "JWKS_URL")

	unset(t, "JWKS_URL")
	_, err = config.Load()
	mustErrNaming(t, err, "JWKS_URL")
}

func TestLoadRefusesEmptyUpstreamAddr(t *testing.T) {
	for _, key := range upstreamKeys {
		t.Run(key, func(t *testing.T) {
			baseEnv(t)
			t.Setenv(key, "")
			_, err := config.Load()
			mustErrNaming(t, err, key)
		})
	}
	baseEnv(t)
	if _, err := config.Load(); err != nil {
		t.Fatalf("Load with default upstreams: %v", err)
	}
}

func TestLoadRefusesInvalidBounds(t *testing.T) {
	for _, tc := range []struct{ key, val string }{
		{"JWKS_CACHE_TTL", "0"},
		{"RETRY_MAX", "-1"},
		{"TRACK_PUBLISH_TIMEOUT", "0s"},
		{"TRACK_PUBLISH_TIMEOUT", "-1s"},
	} {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			baseEnv(t)
			t.Setenv(tc.key, tc.val)
			_, err := config.Load()
			mustErrNaming(t, err, tc.key)
		})
	}
	baseEnv(t)
	t.Setenv("RETRY_MAX", "0")
	if _, err := config.Load(); err != nil {
		t.Errorf("RETRY_MAX=0 must be accepted: %v", err)
	}
}

func TestRetiredKeysAreIgnored(t *testing.T) {
	baseEnv(t)
	t.Setenv("HTTP_PORT", "7000")
	t.Setenv("KAFKA_ENABLED", "false")
	t.Setenv("SHUTDOWN_GRACE_SECONDS", "99")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := number(t, s, "GRPC_PORT"); got != 8080 {
		t.Errorf("GRPC_PORT = %v, want 8080", got)
	}
	if got := duration(t, s, "SHUTDOWN_GRACE"); got != 10*time.Second {
		t.Errorf("SHUTDOWN_GRACE = %v, want 10s", got)
	}
}

func TestEmptyCockpitURLsStayEmpty(t *testing.T) {
	baseEnv(t)
	t.Setenv("PROMETHEUS_URL", "")
	t.Setenv("JAEGER_QUERY_URL", "")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, key := range []string{"PROMETHEUS_URL", "JAEGER_QUERY_URL"} {
		if got := text(t, s, key); got != "" {
			t.Errorf("%s = %q, want empty", key, got)
		}
	}
	if got := text(t, s, "JAEGER_UI_URL"); got != "http://localhost:16686" {
		t.Errorf("JAEGER_UI_URL = %q, want its default", got)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]bool{}
	for _, l := range strings.Split(string(raw), "\n") {
		if k, _, ok := strings.Cut(strings.TrimSpace(l), "="); ok && !strings.HasPrefix(k, "#") {
			lines[k] = true
		}
	}
	for _, k := range []string{
		"GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
		"GRPC_REFLECTION_ENABLED", "MOCK_DATA", "TRACK_PUBLISH_TIMEOUT",
	} {
		if !lines[k] {
			t.Errorf(".env.example does not list %s", k)
		}
	}
	for _, k := range []string{"HTTP_HOST", "HTTP_PORT", "SHUTDOWN_GRACE_SECONDS", "EDGE_REFLECTION_ENABLED", "KAFKA_ENABLED"} {
		if lines[k] {
			t.Errorf(".env.example still lists retired key %s", k)
		}
	}
}

func TestStrictEnvRequiresKafkaBrokers(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production"} {
		t.Run(env, func(t *testing.T) {
			baseEnv(t)
			t.Setenv("ENV", env)
			_, err := config.Load()
			mustErrNaming(t, err, "KAFKA_BROKERS")

			t.Setenv("KAFKA_BROKERS", "kafka:9092")
			if _, err := config.Load(); err != nil {
				t.Errorf("Load with brokers set: %v", err)
			}
		})
	}
}

func TestStrictEnvRefusesMockDataFirst(t *testing.T) {
	baseEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("MOCK_DATA", "cockpit")
	unset(t, "GRPC_PORT")
	_, err := config.Load()
	mustErrNaming(t, err, "MOCK_DATA")
	msg := err.Error()
	if i := strings.Index(msg, "GRPC_PORT"); i >= 0 && i < strings.Index(msg, "MOCK_DATA") {
		t.Errorf("error %q reports GRPC_PORT ahead of MOCK_DATA", msg)
	}
}

func TestStrictEnvRefusesReflection(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production"} {
		t.Run(env, func(t *testing.T) {
			baseEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("KAFKA_BROKERS", "kafka:9092")
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			_, err := config.Load()
			mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
		})
	}
}

func TestRetiredReflectionKeyIsIgnored(t *testing.T) {
	baseEnv(t)
	t.Setenv("EDGE_REFLECTION_ENABLED", "true")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if flag(t, s, "GRPC_REFLECTION_ENABLED") {
		t.Error("EDGE_REFLECTION_ENABLED enabled reflection")
	}
}

func TestEnvOutsideBothSetsRefusesOnlyReflection(t *testing.T) {
	baseEnv(t)
	t.Setenv("ENV", "dev")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load in ENV=dev: %v", err)
	}
	if got := text(t, s, "KAFKA_BROKERS"); got != "localhost:9092" {
		t.Errorf("KAFKA_BROKERS = %q, want localhost:9092", got)
	}

	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err = config.Load()
	mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
}
