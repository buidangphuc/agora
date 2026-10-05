package config

import (
	"os"
	"regexp"
	"testing"
	"time"
)

func TestOutboxDefaultsAndOverrides(t *testing.T) {
	for _, k := range []string{"OUTBOX_ENABLED", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK_SECONDS", "OUTBOX_MAX_ATTEMPTS"} {
		t.Setenv(k, "")
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Outbox; !o.Enabled || o.PollInterval != time.Second || o.BatchSize != 100 || o.ClaimLockSeconds != 60 || o.MaxAttempts != 10 {
		t.Fatalf("defaults = %+v", o)
	}
	t.Setenv("OUTBOX_ENABLED", "false")
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("OUTBOX_BATCH_SIZE", "7")
	s, _ = LoadSettings()
	if o := s.Outbox; o.Enabled || o.PollInterval != 250*time.Millisecond || o.BatchSize != 7 {
		t.Fatalf("overrides = %+v", o)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "garbage")
	if s, _ = LoadSettings(); s.Outbox.PollInterval != time.Second {
		t.Fatalf("bad duration must fall back to 1s, got %v", s.Outbox.PollInterval)
	}
}

// Every env var this package reads must be documented in .env.example, and every
// documented var must still be read: the file cannot drift from the code.
func TestEnvExampleInSync(t *testing.T) {
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`getEnv\("([A-Z0-9_]+)"`).FindAllStringSubmatch(string(src), -1) {
		used[m[1]] = true
	}
	ex, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]+)=`).FindAllStringSubmatch(string(ex), -1) {
		documented[m[1]] = true
	}
	for k := range used {
		if !documented[k] {
			t.Errorf(".env.example is missing %s", k)
		}
	}
	for k := range documented {
		if !used[k] {
			t.Errorf(".env.example documents %s but config.go does not read it", k)
		}
	}
}
