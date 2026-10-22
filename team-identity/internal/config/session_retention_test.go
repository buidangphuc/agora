package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Settings.Retention is the config group for task 4. Fields are read through
// reflection so a missing field fails at runtime (missing behaviour), not at compile time.
//
//	Retention.Session        time.Duration  SESSION_RETENTION
//	Retention.LoginHistory   time.Duration  LOGIN_HISTORY_RETENTION
//	Retention.SweepInterval  time.Duration  SESSION_SWEEP_INTERVAL
//	Retention.SweepBatchSize int            SESSION_SWEEP_BATCH_SIZE
//	Retention.SweepEnabled   bool           SESSION_SWEEP_ENABLED
var retentionKeys = []string{
	"SESSION_RETENTION", "LOGIN_HISTORY_RETENTION", "SESSION_SWEEP_INTERVAL",
	"SESSION_SWEEP_BATCH_SIZE", "SESSION_SWEEP_ENABLED",
}

func retentionField(t *testing.T, s *Settings, name string) reflect.Value {
	t.Helper()
	g := reflect.ValueOf(s).Elem().FieldByName("Retention")
	if !g.IsValid() {
		t.Fatalf("Settings.Retention group missing")
	}
	f := g.FieldByName(name)
	if !f.IsValid() {
		t.Fatalf("Settings.Retention.%s missing", name)
	}
	return f
}

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "dev-rsa-private-key-pem")
	t.Setenv("JWT_KID", "dev-2026")
	for _, k := range retentionKeys {
		os.Unsetenv(k)
	}
}

// Scenario: Session retention: defaults
func TestRetentionDefaults(t *testing.T) {
	baseEnv(t)
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got := retentionField(t, s, "Session").Interface(); got != 720*time.Hour {
		t.Errorf("SESSION_RETENTION default = %v, want 720h", got)
	}
	if got := retentionField(t, s, "LoginHistory").Interface(); got != 4320*time.Hour {
		t.Errorf("LOGIN_HISTORY_RETENTION default = %v, want 4320h", got)
	}
	if got := retentionField(t, s, "SweepInterval").Interface(); got != time.Hour {
		t.Errorf("SESSION_SWEEP_INTERVAL default = %v, want 1h", got)
	}
	if got := retentionField(t, s, "SweepBatchSize").Int(); got != 500 {
		t.Errorf("SESSION_SWEEP_BATCH_SIZE default = %d, want 500", got)
	}
	if !retentionField(t, s, "SweepEnabled").Bool() {
		t.Errorf("SESSION_SWEEP_ENABLED default = false, want true")
	}
}

// Scenario: Session retention: defaults (explicit values are honoured)
func TestRetentionOverrides(t *testing.T) {
	baseEnv(t)
	t.Setenv("SESSION_RETENTION", "5s")
	t.Setenv("LOGIN_HISTORY_RETENTION", "10s")
	t.Setenv("SESSION_SWEEP_INTERVAL", "2s")
	t.Setenv("SESSION_SWEEP_BATCH_SIZE", "7")
	t.Setenv("SESSION_SWEEP_ENABLED", "false")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got := retentionField(t, s, "Session").Interface(); got != 5*time.Second {
		t.Errorf("Session = %v, want 5s", got)
	}
	if got := retentionField(t, s, "LoginHistory").Interface(); got != 10*time.Second {
		t.Errorf("LoginHistory = %v, want 10s", got)
	}
	if got := retentionField(t, s, "SweepInterval").Interface(); got != 2*time.Second {
		t.Errorf("SweepInterval = %v, want 2s", got)
	}
	if got := retentionField(t, s, "SweepBatchSize").Int(); got != 7 {
		t.Errorf("SweepBatchSize = %d, want 7", got)
	}
	if retentionField(t, s, "SweepEnabled").Bool() {
		t.Errorf("SweepEnabled = true, want false")
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
			baseEnv(t)
			t.Setenv(c.env, c.val)
			_, err := LoadSettings()
			if err == nil {
				t.Fatalf("LoadSettings with %s=%s: want error, got nil", c.env, c.val)
			}
			if !strings.Contains(err.Error(), c.env) {
				t.Errorf("error %q does not name %s", err, c.env)
			}
		})
	}
}

// Scenario: Session retention: env example matches config
func TestRetentionEnvExampleHasAllKeys(t *testing.T) {
	declared := map[string]bool{}
	for _, k := range DeclaredEnvKeys() {
		declared[k] = true
	}
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range retentionKeys {
		if !declared[k] {
			t.Errorf("%s not declared in config", k)
		}
		if !strings.Contains(string(raw), k+"=") {
			t.Errorf("%s missing from .env.example", k)
		}
	}
	if err := CheckEnvExample("../../.env.example"); err != nil {
		t.Error(err)
	}
}
