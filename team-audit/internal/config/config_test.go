package config_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-audit/internal/config"
)

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

// TestNoEnableFlagsDeclared: the database and reflection are always on, so
// neither has an enable flag.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, key := range []string{"DATABASE_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(declared, key) {
			t.Errorf("%s is declared; the database and reflection are always on", key)
		}
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "51000")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 51000 {
		t.Fatalf("expected overridden port 51000, got %d", s.Port)
	}
	if s.Postgres.URL != "postgres://x/y" {
		t.Fatalf("expected overridden database url, got %q", s.Postgres.URL)
	}
}

func TestConfigLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestConfigLoadRequiresGRPCPort(t *testing.T) {
	t.Setenv("GRPC_PORT", "") // registers the restore; the variable must then be unset, not empty
	if err := os.Unsetenv("GRPC_PORT"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("err = %v, want an error naming GRPC_PORT", err)
	}
}

// ENV has no default: a deployment that forgets it fails at load instead of
// running as local.
func TestConfigLoadRequiresENV(t *testing.T) {
	t.Setenv("ENV", "") // registers the restore; the variable must then be unset, not empty
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("err = %v, want an error naming ENV", err)
	}
}

// Durations are Go durations and the keys carry no unit suffix.
func TestShutdownDurationsAreGoDurations(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("SHUTDOWN_GRACE", "7s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "250ms")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ShutdownGrace != 7*time.Second || s.DrainDelay != 250*time.Millisecond {
		t.Fatalf("grace %v, drain delay %v; want 7s and 250ms", s.ShutdownGrace, s.DrainDelay)
	}
}
