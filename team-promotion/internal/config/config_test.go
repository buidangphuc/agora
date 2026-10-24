package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-promotion/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "DATABASE_ENABLED", "KAFKA_ENABLED",
	"FEATURE_FLAGS_ENABLED", "OUTBOX_ENABLED", "KAFKA_BROKERS", "SHUTDOWN_GRACE_SECONDS",
	"UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR", "OTEL_ENABLED",
	"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "FLIPT_ADDR", "FEATURE_FLAGS_EVAL_TIMEOUT",
	"FEATURE_FLAGS_EVAL_TIMEOUT_MS", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
	"MAX_AD_BID", "MAX_AD_BUDGET", "PROMOTION_EVENTS_TOPIC",
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
	t.Setenv("GRPC_PORT", "50061")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "domain:50051")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "audit:50066")
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
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	}
	t.Fatalf("unexpected type %T", v)
	return 0
}

func TestLoadRequiresEnv(t *testing.T) {
	// Scenario: Missing ENV is refused
	localEnv(t)
	unset(t, "ENV")
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")

	t.Setenv("ENV", "")
	_, err = config.Load()
	mustErrNaming(t, err, "ENV")
}

func TestLoadRequiresDatabaseURLAndUpstreamAddrs(t *testing.T) {
	// Scenario: Missing database URL or upstream address is refused
	// Scenario: An empty audit address is refused in every ENV
	for _, key := range []string{"DATABASE_URL", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR"} {
		for _, env := range []string{"local", "test", "dev"} {
			t.Run(key+"/"+env+"/empty", func(t *testing.T) {
				localEnv(t)
				t.Setenv("ENV", env)
				t.Setenv(key, "")
				_, err := config.Load()
				mustErrNaming(t, err, key)
			})
		}
		t.Run(key+"/unset", func(t *testing.T) {
			localEnv(t)
			unset(t, key)
			_, err := config.Load()
			mustErrNaming(t, err, key)
		})
	}
	localEnv(t)
	if _, err := config.Load(); err != nil {
		t.Fatalf("fully configured local ENV must load: %v", err)
	}
}

func TestNoDependencyEnableFlags(t *testing.T) {
	// Scenario: Retired enable flags are not read
	allowed := []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED"}
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range declared {
		if strings.HasSuffix(k, "_ENABLED") && !slices.Contains(allowed, k) {
			t.Errorf("Settings declares dependency flag %s", k)
		}
	}
	for _, k := range allowed {
		if !slices.Contains(declared, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}

	localEnv(t)
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("KAFKA_ENABLED", "false")
	t.Setenv("FEATURE_FLAGS_ENABLED", "false")
	t.Setenv("OUTBOX_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestReflectionDefaultOffAndRefusedInStrictEnv(t *testing.T) {
	// Scenario: Reflection is off by default and refused outside local
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ReflectionEnabled {
		t.Error("reflection must default to false")
	}

	for _, env := range []string{"local", "test"} {
		localEnv(t)
		t.Setenv("ENV", env)
		t.Setenv("GRPC_REFLECTION_ENABLED", "true")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("ENV=%s with reflection: %v", env, err)
		}
		if !s.ReflectionEnabled {
			t.Errorf("ENV=%s: reflection = false, want true", env)
		}
	}

	for _, env := range []string{"staging", "prod"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("KAFKA_BROKERS", "kafka:9092")
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			_, err := config.Load()
			mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
		})
	}
}

func TestStrictEnvRequiresKafkaBrokers(t *testing.T) {
	// Scenario: Strict ENV requires brokers
	for _, env := range []string{"staging", "prod"} {
		t.Run(env, func(t *testing.T) {
			localEnv(t)
			t.Setenv("ENV", env)
			t.Setenv("KAFKA_BROKERS", "")
			_, err := config.Load()
			mustErrNaming(t, err, "KAFKA_BROKERS")
		})
	}
	localEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	if _, err := config.Load(); err != nil {
		t.Fatalf("fully configured strict ENV must load: %v", err)
	}
}

func TestAdCapsDefaultsAndValidation(t *testing.T) {
	// Scenario: Ad limits must be positive
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "MAX_AD_BID")); got != 1_000_000 {
		t.Errorf("MAX_AD_BID default = %d", got)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "MAX_AD_BUDGET")); got != 1_000_000_000 {
		t.Errorf("MAX_AD_BUDGET default = %d", got)
	}
	for _, k := range []string{"MAX_AD_BID", "MAX_AD_BUDGET"} {
		for _, v := range []string{"0", "-5", "abc"} {
			t.Run(k+"="+v, func(t *testing.T) {
				localEnv(t)
				t.Setenv(k, v)
				_, err := config.Load()
				mustErrNaming(t, err, k)
			})
		}
		localEnv(t)
		t.Setenv(k, "1")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("%s=1 must load: %v", k, err)
		}
		if got := asInt64(t, settingByEnvKey(t, s, k)); got != 1 {
			t.Errorf("%s = %d, want 1", k, got)
		}
	}
}

func TestRetiredCallTimeoutKeyNotRead(t *testing.T) {
	// Scenario: The retired call timeout key is not read
	if slices.Contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "UPSTREAM_CALL_TIMEOUT_SECONDS") {
		t.Error("Settings declares the retired UPSTREAM_CALL_TIMEOUT_SECONDS")
	}
	localEnv(t)
	t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", "0.01")
	if _, err := config.Load(); err != nil {
		t.Fatalf("retired key must have no effect: %v", err)
	}
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestLoadMigrateNeedsOnlyEnvAndDatabaseURL(t *testing.T) {
	// Scenario: Migrate settings need only ENV and the database URL
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
	for _, k := range []string{"UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR", "KAFKA_BROKERS", "GRPC_PORT"} {
		if slices.Contains(declared, k) {
			t.Errorf("MigrateSettings must not declare %s", k)
		}
	}

	t.Setenv("DATABASE_URL", "")
	_, err = config.LoadMigrate()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestEnvExampleInSync(t *testing.T) {
	// Scenario: .env.example matches the settings
	// Scenario: Env key is declared
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatal(err)
	}
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range []string{"UPSTREAM_AUDIT_ADDR", "UPSTREAM_DOMAIN_ADDR", "PROMOTION_EVENTS_TOPIC", "MAX_AD_BID", "MAX_AD_BUDGET", "FLIPT_ADDR", "OUTBOX_POLL_INTERVAL"} {
		if !slices.Contains(declared, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"GRPC_PORT=50061", "ENV=local", "GRPC_REFLECTION_ENABLED=false"} {
		if !strings.Contains(text, want) {
			t.Errorf(".env.example lacks %q", want)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		for _, k := range []string{"UPSTREAM_AUDIT_ADDR", "UPSTREAM_DOMAIN_ADDR"} {
			if strings.HasPrefix(line, k+"=") && strings.TrimSpace(strings.TrimPrefix(line, k+"=")) == "" {
				t.Errorf("%s must be listed with a value", k)
			}
		}
	}
}
