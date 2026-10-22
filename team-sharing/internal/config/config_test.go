package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-sharing/internal/config"
)

const testDSN = "postgres://test:pass@localhost:5432/db"

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
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

func baseEnv(t *testing.T, env string) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", testDSN)
}

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatalf("scenario Settings are the SDK groups and .env.example matches them: %v", err)
	}
}

func TestConfigLoad(t *testing.T) {
	baseEnv(t, "local")
	t.Setenv("OTEL_SERVICE_NAME", "team-sharing")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Env != "local" {
		t.Errorf("Env = %q, want local", s.Env)
	}
	if s.Port != 50099 {
		t.Errorf("Port = %d, want 50099", s.Port)
	}
	if s.Postgres.URL != testDSN {
		t.Errorf("Postgres.URL = %q", s.Postgres.URL)
	}
	if s.Postgres.MaxConns != 10 {
		t.Errorf("Postgres.MaxConns = %d, want default 10", s.Postgres.MaxConns)
	}
	if s.OTel.ServiceName != "team-sharing" {
		t.Errorf("OTel.ServiceName = %q", s.OTel.ServiceName)
	}
	if s.ShutdownGrace != 10*time.Second {
		t.Errorf("ShutdownGrace = %v, want default 10s", s.ShutdownGrace)
	}
}

func TestConfigLoadRequiresEnvAndPort(t *testing.T) {
	t.Run("missing ENV", func(t *testing.T) {
		baseEnv(t, "local")
		os.Unsetenv("ENV")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
			t.Fatalf("err = %v, want an error naming ENV", err)
		}
	})
	t.Run("empty ENV", func(t *testing.T) {
		baseEnv(t, "")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
			t.Fatalf("err = %v, want an error naming ENV", err)
		}
	})
	t.Run("missing GRPC_PORT", func(t *testing.T) {
		baseEnv(t, "local")
		os.Unsetenv("GRPC_PORT")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
			t.Fatalf("err = %v, want an error naming GRPC_PORT", err)
		}
	})
	t.Run("non-positive GRPC_PORT", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("GRPC_PORT", "0")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
			t.Fatalf("err = %v, want an error naming GRPC_PORT", err)
		}
	})
}

func TestConfigLoadRejectsBadDuration(t *testing.T) {
	baseEnv(t, "local")
	t.Setenv("SHUTDOWN_GRACE", "soon")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "SHUTDOWN_GRACE") {
		t.Fatalf("err = %v, want an error naming SHUTDOWN_GRACE", err)
	}
}

func TestConfigLoadReadsDurations(t *testing.T) {
	baseEnv(t, "local")
	t.Setenv("SHUTDOWN_GRACE", "7s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "250ms")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ShutdownGrace != 7*time.Second || s.DrainDelay != 250*time.Millisecond {
		t.Fatalf("grace = %v, drain = %v", s.ShutdownGrace, s.DrainDelay)
	}
}

func TestLoadRefusesReflectionInStrictEnv(t *testing.T) {
	for _, env := range []string{"staging", "prod", "production", "PROD"} {
		t.Run("scenario Strict ENV refuses reflection "+env, func(t *testing.T) {
			baseEnv(t, env)
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), "GRPC_REFLECTION_ENABLED") {
				t.Fatalf("err = %v, want an error naming GRPC_REFLECTION_ENABLED", err)
			}
		})
	}
	t.Run("local allows reflection", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("GRPC_REFLECTION_ENABLED", "true")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !s.ReflectionEnabled {
			t.Error("ReflectionEnabled = false, want true")
		}
	})
}

func TestLoadRequiresDatabaseURLInEveryEnv(t *testing.T) {
	for _, env := range []string{"local", "test", "staging", "prod"} {
		t.Run("scenario Missing DATABASE_URL is refused "+env, func(t *testing.T) {
			baseEnv(t, env)
			os.Unsetenv("DATABASE_URL")
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
				t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
			}
			t.Setenv("DATABASE_URL", "")
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
				t.Fatalf("empty: err = %v, want an error naming DATABASE_URL", err)
			}
		})
	}
}

func TestLoadHasNoDefaultDSN(t *testing.T) {
	baseEnv(t, "local")
	os.Unsetenv("DATABASE_URL")
	s, err := config.Load()
	if err == nil {
		t.Fatalf("scenario No default DSN is compiled in: Load succeeded with URL %q", s.Postgres.URL)
	}
	if s.Postgres.URL != "" {
		t.Errorf("Postgres.URL = %q, want empty", s.Postgres.URL)
	}
	src, rerr := os.ReadFile("config.go")
	if rerr != nil {
		t.Fatal(rerr)
	}
	for _, needle := range []string{"postgres://", "sharing_pass", "5447"} {
		if strings.Contains(string(src), needle) {
			t.Errorf("config.go still contains %q", needle)
		}
	}
}

func TestDatabaseEnabledFlagIsIgnored(t *testing.T) {
	t.Run("false does not disable the database", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("DATABASE_ENABLED", "false")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("scenario The removed enable flag is ignored: %v", err)
		}
		if s.Postgres.URL != testDSN {
			t.Errorf("Postgres.URL = %q", s.Postgres.URL)
		}
	})
	t.Run("false does not excuse a missing URL", func(t *testing.T) {
		baseEnv(t, "local")
		os.Unsetenv("DATABASE_URL")
		t.Setenv("DATABASE_ENABLED", "false")
		if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
			t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
		}
	})
	t.Run("garbage value does not fail Load", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("DATABASE_ENABLED", "maybe")
		if _, err := config.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})
}
