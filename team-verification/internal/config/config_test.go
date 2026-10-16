package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GRPC_PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg := Load()
	if cfg.GRPCPort != 50064 {
		t.Fatalf("expected default port 50064, got %d", cfg.GRPCPort)
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

// Scenario: Config defaults and overrides.
func TestAuditConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("UPSTREAM_AUDIT_ADDR", "")
	t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.AuditAddr != "" {
		t.Fatalf("default AuditAddr = %q, want empty", cfg.AuditAddr)
	}
	if cfg.CallTimeout != 2*time.Second {
		t.Fatalf("default CallTimeout = %v, want 2s", cfg.CallTimeout)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}

	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", "1.5")
	cfg = Load()
	if cfg.AuditAddr != "team-audit-svc:50066" {
		t.Fatalf("AuditAddr = %q", cfg.AuditAddr)
	}
	if cfg.CallTimeout != 1500*time.Millisecond {
		t.Fatalf("CallTimeout = %v, want 1.5s", cfg.CallTimeout)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("override must validate: %v", err)
	}
}

// Scenario: Non-positive call timeout is rejected.
func TestCallTimeoutMustBePositive(t *testing.T) {
	for _, raw := range []string{"0", "-1"} {
		t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", raw)
		if err := Load().Validate(); err == nil {
			t.Fatalf("UPSTREAM_CALL_TIMEOUT_SECONDS=%s must be rejected", raw)
		}
	}
}
