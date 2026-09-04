package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-notification/internal/config"
)

func TestConfigLoad(t *testing.T) {
	os.Setenv("GRPC_PORT", "50099")
	os.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	os.Setenv("KAFKA_BROKER", "localhost:9092")
	defer func() {
		os.Unsetenv("GRPC_PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("KAFKA_BROKER")
	}()

	cfg := config.Load()
	if cfg.GRPCPort != 50099 {
		t.Errorf("expected 50099, got %d", cfg.GRPCPort)
	}
	if cfg.DatabaseURL != "postgres://test:pass@localhost:5432/db" {
		t.Errorf("unexpected database URL")
	}
	if cfg.KafkaBroker != "localhost:9092" {
		t.Errorf("unexpected kafka broker")
	}
}
