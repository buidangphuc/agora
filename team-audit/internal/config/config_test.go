package config_test

import (
	"testing"

	"github.com/buidangphuc/team-audit/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GRPC_PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg := config.Load()
	if cfg.GRPCPort != 50065 {
		t.Fatalf("expected default port 50065, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL == "" {
		t.Fatalf("expected a default database url")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("GRPC_PORT", "51000")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	cfg := config.Load()
	if cfg.GRPCPort != 51000 {
		t.Fatalf("expected overridden port 51000, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL != "postgres://x/y" {
		t.Fatalf("expected overridden database url, got %q", cfg.DatabaseURL)
	}
}
