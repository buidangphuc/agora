package config_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-verification/internal/config"
)

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestNoEnableFlagsDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, flag := range []string{"DATABASE_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(keys, flag) {
			t.Errorf("%s is declared; always-on capabilities have no enable flag", flag)
		}
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "51000")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 51000 {
		t.Fatalf("expected port 51000, got %d", s.Port)
	}
	if s.Postgres.URL != "postgres://x/y" {
		t.Fatalf("expected overridden DATABASE_URL, got %s", s.Postgres.URL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50064")
	t.Setenv("DATABASE_URL", "")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestLoadRequiresGRPCPort(t *testing.T) {
	t.Setenv("GRPC_PORT", "") // registers the restore; the variable must be unset, not empty
	if err := os.Unsetenv("GRPC_PORT"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("err = %v, want an error naming GRPC_PORT", err)
	}
}

func TestLoadRequiresEnv(t *testing.T) {
	t.Setenv("ENV", "") // registers the restore; the variable must be unset
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRPC_PORT", "50064")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("err = %v, want an error naming ENV", err)
	}
}

func TestShutdownDurationsAreGoDurations(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50064")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("SHUTDOWN_GRACE", "7s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "500ms")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ShutdownGrace != 7*time.Second || s.DrainDelay != 500*time.Millisecond {
		t.Fatalf("grace = %v, drain = %v", s.ShutdownGrace, s.DrainDelay)
	}
}
