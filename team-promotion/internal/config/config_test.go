package config_test

import (
	"os"
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-promotion/internal/config"
)

func TestConfigLoadSettings(t *testing.T) {
	os.Setenv("ENV", "test")
	os.Setenv("GRPC_PORT", "50060")
	os.Setenv("DATABASE_URL", "postgres://x/y")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Runtime.Env != "test" {
		t.Errorf("expected Env test, got %s", cfg.Runtime.Env)
	}
	if cfg.Server.Port != 50060 {
		t.Errorf("expected Port 50060, got %d", cfg.Server.Port)
	}
}

func TestAdCapsAndTimeoutDefaultsAndValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("GRPC_PORT", "50061")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ads.MaxBid != 1_000_000 || cfg.Ads.MaxBudget != 1_000_000_000 || cfg.Upstream.CallTimeoutSeconds != 2 {
		t.Errorf("defaults = %d/%d/%v", cfg.Ads.MaxBid, cfg.Ads.MaxBudget, cfg.Upstream.CallTimeoutSeconds)
	}
	for _, k := range []string{"MAX_AD_BID", "MAX_AD_BUDGET", "UPSTREAM_CALL_TIMEOUT_SECONDS"} {
		for _, v := range []string{"0", "-5", "abc"} {
			t.Setenv(k, v)
			if _, err := config.Load(); err == nil {
				t.Errorf("%s=%s accepted, want error", k, v)
			}
		}
		t.Setenv(k, "5")
	}
}

// TestDeclaredEnvKeysNoDrift guards against config/.env.example drift: every key
// declared in Settings must be documented in .env.example (and vice-versa).
func TestDeclaredEnvKeysNoDrift(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	documented := map[string]bool{}
	for _, line := range splitLines(string(raw)) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if i := indexByte(line, '='); i > 0 {
			documented[line[:i]] = true
		}
	}
	for _, key := range config.DeclaredEnvKeys() {
		if !documented[key] {
			t.Errorf("declared env key %q missing from .env.example", key)
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// TestEnvExampleInSync is the both-directions env-drift gate (`make check-env`).
func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

// TestNoEnableFlagsDeclared: capabilities every deploy runs have no enable flag.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := config.DeclaredEnvKeys()
	for _, key := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(declared, key) {
			t.Errorf("%s is declared; the database, the Kafka publisher, the outbox relayer and reflection are always on", key)
		}
	}
}

// TestFeatureFlagsToggleKept: FEATURE_FLAGS_ENABLED is a real toggle (gitops runs
// promotion with it false), so it stays declared and defaults to on.
func TestFeatureFlagsToggleKept(t *testing.T) {
	if !slices.Contains(config.DeclaredEnvKeys(), "FEATURE_FLAGS_ENABLED") {
		t.Fatal("FEATURE_FLAGS_ENABLED must stay declared")
	}
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("GRPC_PORT", "50061")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Flags.Enabled {
		t.Error("FEATURE_FLAGS_ENABLED must default to true")
	}
}
