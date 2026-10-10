package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/config"
)

func TestConfigLoadSettings(t *testing.T) {
	os.Setenv("ENV", "test")
	os.Setenv("GRPC_PORT", "50055")
	os.Setenv("DATABASE_ENABLED", "false")

	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Runtime.Env != "test" {
		t.Errorf("expected Env test, got %s", cfg.Runtime.Env)
	}
	if cfg.Server.Port != 50055 {
		t.Errorf("expected Port 50055, got %d", cfg.Server.Port)
	}
}

// TestEnvExampleInSync is the env-drift gate: .env.example must document exactly
// the keys Settings declares (both directions).
func TestEnvExampleInSync(t *testing.T) {
	if err := config.CheckEnvExample("../../.env.example"); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxAndKafkaDefaults(t *testing.T) {
	for _, k := range []string{"KAFKA_ENABLED", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "ORDER_EVENTS_TOPIC"} {
		os.Unsetenv(k)
	}
	os.Setenv("DATABASE_ENABLED", "false")
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.Enabled || cfg.Kafka.OrderTopic != "order.events" {
		t.Errorf("kafka defaults wrong: %+v", cfg.Kafka)
	}
	if cfg.OutboxPollInterval().String() != "1s" || cfg.Outbox.BatchSize != 100 || !cfg.Outbox.Enabled {
		t.Errorf("outbox defaults wrong: %+v", cfg.Outbox)
	}
}

func TestRequireDurableStorage(t *testing.T) {
	cases := []struct {
		env       string
		dbEnabled bool
		pool      bool
		wantErr   bool
	}{
		{"local", false, false, false},
		{"test", true, false, false},
		{"", false, false, false},
		{"staging", false, false, true},
		{"staging", true, false, true},
		{"staging", true, true, false},
		{"prod", false, false, true},
		{" Production ", true, false, true},
		{"production", true, true, false},
	}
	for _, tc := range cases {
		s := &config.Settings{
			Runtime:  config.Runtime{Env: tc.env},
			Database: config.Database{Enabled: tc.dbEnabled},
		}
		err := s.RequireDurableStorage(tc.pool)
		if (err != nil) != tc.wantErr {
			t.Errorf("ENV=%q enabled=%v pool=%v: err=%v wantErr=%v", tc.env, tc.dbEnabled, tc.pool, err, tc.wantErr)
		}
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
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("RESERVATION_TTL", "banana")
	os.Unsetenv("RESERVATION_SWEEP_INTERVAL")
	cfg, err := config.LoadSettings()
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
