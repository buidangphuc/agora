package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-order/internal/config"
)

func TestConfigLoadSettings(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Env != "test" {
		t.Errorf("expected Env test, got %s", cfg.Env)
	}
	if cfg.Port != 50055 {
		t.Errorf("expected Port 50055, got %d", cfg.Port)
	}
}

// TestEnvExampleInSync is the env-drift gate: .env.example must document exactly
// the keys Settings declares (both directions).
func TestEnvExampleInSync(t *testing.T) {
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxAndKafkaDefaults(t *testing.T) {
	for _, k := range []string{"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "ORDER_EVENTS_TOPIC"} {
		os.Unsetenv(k)
	}
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Events.OrderTopic != "order.events" {
		t.Errorf("kafka defaults wrong: %+v", cfg.Events)
	}
	if cfg.Outbox.PollInterval.String() != "1s" || cfg.Outbox.BatchSize != 100 {
		t.Errorf("outbox defaults wrong: %+v", cfg.Outbox)
	}
}

// ENV has no default: a deployment that forgets it fails at load instead of
// running as local.
func TestLoadRequiresEnv(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("Load without ENV = %v, want an error naming ENV", err)
	}
}

// An explicitly empty duration key stays empty (keepempty), so it reaches the
// fallback with a warning instead of silently taking the default.
func TestExplicitlyEmptyReservationKeyWarns(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("RESERVATION_TTL", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if d, w := cfg.ReservationTTL(); d != 15*time.Minute || !strings.Contains(w, "RESERVATION_TTL") {
		t.Fatalf("ttl=%s warn=%q, want default with a warning naming the key", d, w)
	}
}

func TestReservationDurations(t *testing.T) {
	cases := []struct {
		name             string
		ttl, interval    string
		wantTTL, wantInt time.Duration
		wantWarnTTL      bool
		wantWarnInterval bool
	}{
		{"defaults", "15m", "1m", 15 * time.Minute, time.Minute, false, false},
		{"override", "20s", "2s", 20 * time.Second, 2 * time.Second, false, false},
		{"invalid", "banana", "0s", 15 * time.Minute, time.Minute, true, true},
		{"negative and empty", "-1m", "", 15 * time.Minute, time.Minute, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &config.Settings{Reservation: config.Reservation{TTL: tc.ttl, SweepInterval: tc.interval}}
			ttl, w1 := s.ReservationTTL()
			iv, w2 := s.ReservationSweepInterval()
			if ttl != tc.wantTTL || iv != tc.wantInt {
				t.Fatalf("got ttl=%s interval=%s, want %s/%s", ttl, iv, tc.wantTTL, tc.wantInt)
			}
			if (w1 != "") != tc.wantWarnTTL || (w2 != "") != tc.wantWarnInterval {
				t.Fatalf("warnings ttl=%q interval=%q", w1, w2)
			}
			if tc.wantWarnTTL && !strings.Contains(w1, "RESERVATION_TTL") {
				t.Fatalf("ttl warning must name the variable: %q", w1)
			}
			if tc.wantWarnInterval && !strings.Contains(w2, "RESERVATION_SWEEP_INTERVAL") {
				t.Fatalf("interval warning must name the variable: %q", w2)
			}
		})
	}
}

func TestReservationDefaultsFromEnv(t *testing.T) {
	t.Setenv("ENV", "test")
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("RESERVATION_TTL", "banana")
	os.Unsetenv("RESERVATION_SWEEP_INTERVAL")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("an invalid RESERVATION_TTL must not fail config loading: %v", err)
	}
	if d, w := cfg.ReservationTTL(); d != 15*time.Minute || w == "" {
		t.Fatalf("ttl=%s warn=%q", d, w)
	}
	if d, w := cfg.ReservationSweepInterval(); d != time.Minute || w != "" {
		t.Fatalf("unset interval: %s warn=%q", d, w)
	}
}
