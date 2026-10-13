package config_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-referral/internal/config"
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
			t.Errorf("%s is declared; always-on capabilities take no enable flag", flag)
		}
	}
}

func TestConfigLoad(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 50099 {
		t.Errorf("expected 50099, got %d", s.Port)
	}
	if s.Postgres.URL != "postgres://test:pass@localhost:5432/db" {
		t.Errorf("unexpected database URL %q", s.Postgres.URL)
	}
}

func TestConfigLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

// ENV has no default: a deployment that forgets it must fail at load rather
// than run as local.
func TestConfigLoadRequiresEnv(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("err = %v, want an error naming ENV", err)
	}
}

// Shutdown timings are Go durations under keys without a unit suffix.
func TestShutdownTimingsAreDurations(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	t.Setenv("SHUTDOWN_GRACE", "7s")
	t.Setenv("SHUTDOWN_DRAIN_DELAY", "250ms")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ShutdownGrace != 7*time.Second || s.DrainDelay != 250*time.Millisecond {
		t.Errorf("grace = %v, drain = %v, want 7s and 250ms", s.ShutdownGrace, s.DrainDelay)
	}
}
