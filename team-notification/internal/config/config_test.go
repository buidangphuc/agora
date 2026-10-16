package config_test

import (
	"os"
	"strings"
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

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
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

func TestUpstreamAddrs(t *testing.T) {
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DomainAddr != "localhost:50051" || cfg.IdentityAddr != "localhost:50053" {
		t.Errorf("defaults = %q / %q", cfg.DomainAddr, cfg.IdentityAddr)
	}
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "team-domain-svc:50051")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "team-identity-svc:50053")
	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DomainAddr != "team-domain-svc:50051" || cfg.IdentityAddr != "team-identity-svc:50053" {
		t.Errorf("overrides = %q / %q", cfg.DomainAddr, cfg.IdentityAddr)
	}
}

// Every env var config.Load reads must be documented in .env.example.
func TestEnvExampleListsConfigVars(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	for _, k := range []string{"GRPC_PORT", "DATABASE_URL", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR", "LOW_STOCK_THRESHOLD"} {
		if !strings.Contains(string(raw), k+"=") {
			t.Errorf(".env.example is missing %s", k)
		}
	}
}

func TestLoadLowStockThreshold(t *testing.T) {
	// Scenario: Order work: The threshold is configurable
	t.Setenv("LOW_STOCK_THRESHOLD", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("default Load: %v", err)
	}
	if cfg.LowStockThreshold != 5 {
		t.Errorf("default threshold = %d, want 5", cfg.LowStockThreshold)
	}
	for env, want := range map[string]int{"10": 10, "0": 0} {
		t.Setenv("LOW_STOCK_THRESHOLD", env)
		cfg, err = config.Load()
		if err != nil {
			t.Fatalf("Load(%q): %v", env, err)
		}
		if int(cfg.LowStockThreshold) != want {
			t.Errorf("threshold(%q) = %d, want %d", env, cfg.LowStockThreshold, want)
		}
	}
}

func TestLoadLowStockThresholdInvalid(t *testing.T) {
	// Scenario: Order work: An invalid threshold value fails startup
	for _, bad := range []string{"-1", "abc", "1.5", "99999999999"} {
		t.Setenv("LOW_STOCK_THRESHOLD", bad)
		if _, err := config.Load(); err == nil {
			t.Errorf("Load(%q) returned nil error", bad)
		}
	}
}
