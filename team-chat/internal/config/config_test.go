package config_test

import (
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-chat/internal/config"
)

func TestOutboxDefaultsAndOverrides(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50057")
	t.Setenv("DATABASE_URL", "postgres://test:pass@localhost:5432/db")
	for _, k := range []string{"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK"} {
		t.Setenv(k, "")
	}
	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Outbox; o.PollInterval != time.Second || o.BatchSize != 100 || o.ClaimLock != 60*time.Second {
		t.Fatalf("defaults = %+v", o)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("OUTBOX_BATCH_SIZE", "7")
	s, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Outbox; o.PollInterval != 250*time.Millisecond || o.BatchSize != 7 {
		t.Fatalf("overrides = %+v", o)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "garbage")
	if _, err = config.Load(); err == nil || !strings.Contains(err.Error(), "OUTBOX_POLL_INTERVAL") {
		t.Fatalf("a bad duration must fail the load naming OUTBOX_POLL_INTERVAL, got %v", err)
	}
}

// Every env var Settings declares must be documented in .env.example, and every
// documented var must still be declared: the file cannot drift from the code.
func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}
