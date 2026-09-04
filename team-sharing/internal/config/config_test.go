package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-sharing/internal/config"
)

func TestConfigLoad(t *testing.T) {
	os.Setenv("GRPC_PORT", "50099")
	os.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	defer func() {
		os.Unsetenv("GRPC_PORT")
		os.Unsetenv("DATABASE_URL")
	}()

	cfg := config.Load()
	if cfg.GRPCPort != 50099 {
		t.Errorf("expected 50099, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL != "postgres://test:pass@localhost:5432/db" {
		t.Errorf("unexpected database URL")
	}
}

func TestConfigDefaults(t *testing.T) {
	os.Unsetenv("GRPC_PORT")
	os.Unsetenv("DATABASE_URL")
	cfg := config.Load()
	if cfg.GRPCPort != 50065 {
		t.Errorf("expected default port 50065, got %d", cfg.GRPCPort)
	}
}
