package config_test

import (
	"slices"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-identity/internal/config"
)

// Scenario: Session retention: defaults
func TestRetentionDefaults(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if got := valueByEnv(t, s, "SESSION_RETENTION"); got != 720*time.Hour {
		t.Errorf("SESSION_RETENTION = %v, want 720h", got)
	}
	if got := valueByEnv(t, s, "LOGIN_HISTORY_RETENTION"); got != 4320*time.Hour {
		t.Errorf("LOGIN_HISTORY_RETENTION = %v, want 4320h", got)
	}
	if got := valueByEnv(t, s, "SESSION_SWEEP_INTERVAL"); got != time.Hour {
		t.Errorf("SESSION_SWEEP_INTERVAL = %v, want 1h", got)
	}
	if got := valueByEnv(t, s, "SESSION_SWEEP_BATCH_SIZE"); got != 500 {
		t.Errorf("SESSION_SWEEP_BATCH_SIZE = %v, want 500", got)
	}
}

// Scenario: Session retention: defaults (explicit values are honoured)
func TestRetentionOverrides(t *testing.T) {
	localEnv(t)
	t.Setenv("SESSION_RETENTION", "5s")
	t.Setenv("LOGIN_HISTORY_RETENTION", "10s")
	t.Setenv("SESSION_SWEEP_INTERVAL", "2s")
	t.Setenv("SESSION_SWEEP_BATCH_SIZE", "7")
	s := mustLoad(t)
	if got := valueByEnv(t, s, "SESSION_RETENTION"); got != 5*time.Second {
		t.Errorf("SESSION_RETENTION = %v, want 5s", got)
	}
	if got := valueByEnv(t, s, "LOGIN_HISTORY_RETENTION"); got != 10*time.Second {
		t.Errorf("LOGIN_HISTORY_RETENTION = %v, want 10s", got)
	}
	if got := valueByEnv(t, s, "SESSION_SWEEP_INTERVAL"); got != 2*time.Second {
		t.Errorf("SESSION_SWEEP_INTERVAL = %v, want 2s", got)
	}
	if got := valueByEnv(t, s, "SESSION_SWEEP_BATCH_SIZE"); got != 7 {
		t.Errorf("SESSION_SWEEP_BATCH_SIZE = %v, want 7", got)
	}
}

// Scenario: Session retention: invalid value is refused
func TestRetentionInvalidRejected(t *testing.T) {
	cases := []struct{ env, val string }{
		{"SESSION_RETENTION", "0"},
		{"SESSION_RETENTION", "abc"},
		{"SESSION_RETENTION", "-1h"},
		{"LOGIN_HISTORY_RETENTION", "0"},
		{"LOGIN_HISTORY_RETENTION", "abc"},
		{"SESSION_SWEEP_INTERVAL", "0"},
		{"SESSION_SWEEP_INTERVAL", "abc"},
		{"SESSION_SWEEP_BATCH_SIZE", "0"},
		{"SESSION_SWEEP_BATCH_SIZE", "-5"},
		{"SESSION_SWEEP_BATCH_SIZE", "abc"},
	}
	for _, c := range cases {
		t.Run(c.env+"="+c.val, func(t *testing.T) {
			localEnv(t)
			t.Setenv(c.env, c.val)
			_, err := config.Load()
			mustContain(t, err, c.env)
		})
	}
}

// Scenario: Session retention: disabled sweeper
func TestRetentionHasNoEnableFlag(t *testing.T) {
	localEnv(t)
	t.Setenv("SESSION_SWEEP_ENABLED", "false")
	mustLoad(t)
	if slices.Contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "SESSION_SWEEP_ENABLED") {
		t.Error("SESSION_SWEEP_ENABLED must not be declared")
	}
}
