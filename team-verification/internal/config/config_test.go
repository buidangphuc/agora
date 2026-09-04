package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GRPC_PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg := Load()
	if cfg.GRPCPort != 50062 {
		t.Fatalf("expected default port 50062, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL == "" {
		t.Fatalf("expected a default DATABASE_URL")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GRPC_PORT", "51000")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	cfg := Load()
	if cfg.GRPCPort != 51000 {
		t.Fatalf("expected port 51000, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL != "postgres://x/y" {
		t.Fatalf("expected overridden DATABASE_URL, got %s", cfg.DatabaseURL)
	}
}
