package config_test

import (
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-sharing/internal/config"
)

func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestConfigLoad(t *testing.T) {
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
	t.Setenv("GRPC_PORT", "50099")
	t.Setenv("DATABASE_URL", "")

	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}
