package config_test

import (
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/config"
)

const handoffTTLKey = "HANDOFF_TTL_OVERRIDE"

// Scenario: Override accepted in test
func TestTTLOverrideOnlyInTestAccepted(t *testing.T) {
	localEnv(t)
	t.Setenv("ENV", "test")
	t.Setenv(handoffTTLKey, "5s")
	if got := valueByEnv(t, mustLoad(t), handoffTTLKey); got != 5*time.Second {
		t.Fatalf("%s = %v, want 5s", handoffTTLKey, got)
	}
}

// Scenario: Default TTL without override
func TestTTLOverrideDefaultsToNone(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		localEnv(t)
		t.Setenv("ENV", env)
		if got := valueByEnv(t, mustLoad(t), handoffTTLKey); got != time.Duration(0) {
			t.Fatalf("ENV=%s: %s = %v, want no override (0)", env, handoffTTLKey, got)
		}
	}
}

// Scenario: Override refused outside test
func TestTTLOverrideOnlyInTestRefusedElsewhere(t *testing.T) {
	for _, env := range []string{"local", "staging", "stage", "prod", "production", "Production"} {
		strictEnv(t, env)
		t.Setenv(handoffTTLKey, "5s")
		_, err := config.Load()
		mustContain(t, err, handoffTTLKey)
	}
}
