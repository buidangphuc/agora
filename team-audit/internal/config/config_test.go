package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-audit/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "MOCK_DATA", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
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

func TestLoadReadsEnvironment(t *testing.T) {
	// scenario: Load reads the environment
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "51000")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 51000 {
		t.Errorf("Port = %d, want 51000", s.Port)
	}
	if s.Postgres.URL != "postgres://x/y" {
		t.Errorf("Postgres.URL = %q, want postgres://x/y", s.Postgres.URL)
	}
	if s.Postgres.MaxConns != 10 {
		t.Errorf("Postgres.MaxConns = %d, want 10", s.Postgres.MaxConns)
	}
	if s.OTel.Enabled {
		t.Error("OTel.Enabled = true, want false")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// scenario: A missing database URL fails the load
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50066")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestLoadRequiresGRPCPort(t *testing.T) {
	// scenario: A missing port or environment fails the load
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("GRPC_PORT", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("empty port: err = %v, want an error naming GRPC_PORT", err)
	}

	cleanEnv(t)
	t.Setenv("ENV", "")
	t.Setenv("GRPC_PORT", "50066")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("empty ENV: err = %v, want an error naming ENV", err)
	}
}

func TestStrictEnvRefusesReflectionBeforeValidation(t *testing.T) {
	// scenario: Strict ENV refuses reflection before validation
	cleanEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("GRPC_PORT", "50066")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded in production with reflection and no DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "GRPC_REFLECTION_ENABLED") {
		t.Errorf("err = %v, want the reflection refusal", err)
	}
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("err = %v, must not be the DATABASE_URL error", err)
	}
}

func TestReflectionDefaultsOffAndLocalAccepts(t *testing.T) {
	// scenario: Reflection is off by default and accepted locally
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50066")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ReflectionEnabled {
		t.Error("ReflectionEnabled = true by default, want false")
	}
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("Load with reflection: %v", err)
	}
	if !s.ReflectionEnabled {
		t.Error("ReflectionEnabled = false, want true")
	}
}

func TestNoDependencyEnableFlags(t *testing.T) {
	// scenario: No dependency enable flag is declared
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if len(keys) == 0 {
		t.Fatal("Settings declares no keys")
	}
	var got []string
	for _, k := range keys {
		if k == "DATABASE_ENABLED" || k == "MOCK_DATA" {
			t.Errorf("Settings declares %s", k)
		}
		if strings.HasSuffix(k, "_ENABLED") {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	want := []string{"GRPC_REFLECTION_ENABLED", "OTEL_ENABLED"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("declared enable flags = %v, want %v", got, want)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	// scenario: .env.example matches the declared keys
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatalf("CheckEnvExample: %v", err)
	}
}
