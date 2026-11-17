package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/config"
)

// Scenario: Request: cancel window and sweep defaults
func TestAccountDeletionDefaults(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if got := valueByEnv(t, s, "ACCOUNT_DELETION_CANCEL_WINDOW"); got != 48*time.Hour {
		t.Errorf("ACCOUNT_DELETION_CANCEL_WINDOW = %v, want 48h", got)
	}
	if got := valueByEnv(t, s, "ACCOUNT_ERASURE_SWEEP_INTERVAL"); got != time.Hour {
		t.Errorf("ACCOUNT_ERASURE_SWEEP_INTERVAL = %v, want 1h", got)
	}
	if got := valueByEnv(t, s, "UPSTREAM_ORDER_ADDR"); got != "order:50055" {
		t.Errorf("UPSTREAM_ORDER_ADDR = %v", got)
	}
	if got := valueByEnv(t, s, "UPSTREAM_PAYMENT_ADDR"); got != "payment:50056" {
		t.Errorf("UPSTREAM_PAYMENT_ADDR = %v", got)
	}
}

// Scenario: Sweeper: explicit window and interval are honoured (test-only short window)
func TestAccountDeletionOverrides(t *testing.T) {
	localEnv(t)
	t.Setenv("ACCOUNT_DELETION_CANCEL_WINDOW", "3s")
	t.Setenv("ACCOUNT_ERASURE_SWEEP_INTERVAL", "2s")
	s := mustLoad(t)
	if got := valueByEnv(t, s, "ACCOUNT_DELETION_CANCEL_WINDOW"); got != 3*time.Second {
		t.Errorf("window = %v, want 3s", got)
	}
	if got := valueByEnv(t, s, "ACCOUNT_ERASURE_SWEEP_INTERVAL"); got != 2*time.Second {
		t.Errorf("interval = %v, want 2s", got)
	}
}

// Scenario: Request: obligations unreachable (boot fails when an upstream address is missing)
func TestObligationUpstreamsRequiredInEveryEnv(t *testing.T) {
	for _, env := range []string{"local", "production"} {
		for _, key := range []string{"UPSTREAM_ORDER_ADDR", "UPSTREAM_PAYMENT_ADDR"} {
			strictEnv(t, env)
			unset(t, key)
			_, err := config.Load()
			mustContain(t, err, key)

			t.Setenv(key, "")
			_, err = config.Load()
			mustContain(t, err, key)
		}
	}
}

// Scenario: Request: invalid cancel window or sweep interval is refused
func TestAccountDeletionInvalidRejected(t *testing.T) {
	cases := []struct{ env, val string }{
		{"ACCOUNT_DELETION_CANCEL_WINDOW", "0"},
		{"ACCOUNT_DELETION_CANCEL_WINDOW", "abc"},
		{"ACCOUNT_DELETION_CANCEL_WINDOW", "-1h"},
		{"ACCOUNT_ERASURE_SWEEP_INTERVAL", "0"},
		{"ACCOUNT_ERASURE_SWEEP_INTERVAL", "abc"},
		{"ACCOUNT_ERASURE_SWEEP_INTERVAL", "-1h"},
	}
	for _, c := range cases {
		localEnv(t)
		t.Setenv(c.env, c.val)
		_, err := config.Load()
		mustContain(t, err, c.env)
	}
}

// Scenario: Request: obligations unreachable (compose gives identity the new keys)
func TestComposeIdentityCarriesAccountDeletionKeys(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.services.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)\n  team-identity:\n.*?\n  team-domain:`).Find(raw)
	if m == nil {
		t.Fatal("team-identity service block not found")
	}
	block := string(m)
	for _, want := range []string{"UPSTREAM_ORDER_ADDR=team-order-svc:50055", "UPSTREAM_PAYMENT_ADDR=team-payment-svc:50056"} {
		if !strings.Contains(block, want) {
			t.Errorf("identity compose env missing %s", want)
		}
	}
	for _, key := range []string{"ACCOUNT_DELETION_CANCEL_WINDOW", "ACCOUNT_ERASURE_SWEEP_INTERVAL"} {
		if !strings.Contains(block, key) {
			t.Errorf("identity compose env missing %s", key)
		}
	}
}
