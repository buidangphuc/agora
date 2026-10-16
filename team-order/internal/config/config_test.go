package config_test

import (
	"os"
	"reflect"
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

// Scenario: Order audit: Config exposes the audit address
func TestUpstreamAuditConfig(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "false")
	load := func() reflect.Value {
		t.Helper()
		cfg, err := config.LoadSettings()
		if err != nil {
			t.Fatal(err)
		}
		return reflect.ValueOf(cfg.Upstream)
	}
	field := func(up reflect.Value, name string) reflect.Value {
		t.Helper()
		f := up.FieldByName(name)
		if !f.IsValid() {
			t.Fatalf("Upstream.%s is not declared", name)
		}
		return f
	}

	os.Unsetenv("UPSTREAM_AUDIT_ADDR")
	os.Unsetenv("UPSTREAM_CALL_TIMEOUT_SECONDS")
	up := load()
	if got := field(up, "AuditAddr").String(); got != "" {
		t.Fatalf("AuditAddr default = %q, want empty", got)
	}
	if got := field(up, "CallTimeoutSeconds").Int(); got != 2 {
		t.Fatalf("CallTimeoutSeconds default = %d, want 2", got)
	}

	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	if got := field(load(), "AuditAddr").String(); got != "team-audit-svc:50066" {
		t.Fatalf("AuditAddr = %q", got)
	}
}

type lifecycleAccessor struct {
	env string
	def time.Duration
	get func(*config.Settings) (time.Duration, string)
}

func lifecycleAccessors() []lifecycleAccessor {
	return []lifecycleAccessor{
		{"PAYMENT_DEADLINE", 24 * time.Hour, (*config.Settings).PaymentDeadline},
		{"PAYMENT_REMINDER_BEFORE", 2 * time.Hour, (*config.Settings).PaymentReminderBefore},
		{"LIFECYCLE_SWEEP_INTERVAL", time.Minute, (*config.Settings).LifecycleSweepInterval},
	}
}

func loadLifecycle(t *testing.T) *config.Settings {
	t.Helper()
	t.Setenv("DATABASE_ENABLED", "false")
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("startup must not fail: %v", err)
	}
	return cfg
}

// Scenario: Order expiry: Defaults apply when nothing is configured
func TestLifecycleDefaults(t *testing.T) {
	for _, a := range lifecycleAccessors() {
		os.Unsetenv(a.env)
	}
	cfg := loadLifecycle(t)
	for _, a := range lifecycleAccessors() {
		d, warn := a.get(cfg)
		if d != a.def || warn != "" {
			t.Errorf("%s: got (%s, %q), want (%s, no warning)", a.env, d, warn, a.def)
		}
	}
}

// Scenario: Order expiry: Invalid duration falls back with a warning
func TestLifecycleUnusableDurationFallsBack(t *testing.T) {
	for _, a := range lifecycleAccessors() {
		for _, bad := range []string{"0", "-5m", "soon", "", "0s"} {
			t.Run(a.env+"="+bad, func(t *testing.T) {
				t.Setenv(a.env, bad)
				cfg := loadLifecycle(t)
				d, warn := a.get(cfg)
				if d != a.def {
					t.Errorf("got %s, want default %s", d, a.def)
				}
				if !strings.Contains(warn, a.env) {
					t.Errorf("warning %q must name %s", warn, a.env)
				}
			})
		}
	}
}

// Scenario: Order expiry: Invalid duration falls back with a warning (valid values are honoured, no warning)
func TestLifecycleValidDurationHonoured(t *testing.T) {
	vals := map[string]time.Duration{"PAYMENT_DEADLINE": 45 * time.Second, "PAYMENT_REMINDER_BEFORE": 30 * time.Second, "LIFECYCLE_SWEEP_INTERVAL": 2 * time.Second}
	for k, v := range vals {
		t.Setenv(k, v.String())
	}
	cfg := loadLifecycle(t)
	for _, a := range lifecycleAccessors() {
		d, warn := a.get(cfg)
		if d != vals[a.env] || warn != "" {
			t.Errorf("%s: got (%s, %q), want (%s, no warning)", a.env, d, warn, vals[a.env])
		}
	}
}

func receiptDurationEnvs() []string {
	return []string{"AUTO_COMPLETE_AFTER", "RETURN_WINDOW", "RETURN_AUTO_APPROVE_AFTER", "SHIP_BY_AFTER"}
}

// setValidReceiptEnvs pins all four variables to valid values; callers then
// override the one under test.
func setValidReceiptEnvs(t *testing.T) {
	t.Helper()
	for _, k := range receiptDurationEnvs() {
		t.Setenv(k, "1h")
	}
}

// Scenario: Order receipt: Defaults apply when nothing is set
func TestLifecycleDurationDefaults(t *testing.T) {
	for _, k := range receiptDurationEnvs() {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("startup must not fail with defaults: %v", err)
	}
	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"AUTO_COMPLETE_AFTER", cfg.AutoCompleteAfter(), 168 * time.Hour},
		{"RETURN_WINDOW", cfg.ReturnWindow(), 360 * time.Hour},
		{"RETURN_AUTO_APPROVE_AFTER", cfg.ReturnAutoApproveAfter(), 72 * time.Hour},
		{"SHIP_BY_AFTER", cfg.ShipByAfter(), 48 * time.Hour},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, c.got, c.want)
		}
	}
}

// Scenario: Order receipt: Defaults apply when nothing is set (valid values are honoured)
func TestLifecycleDurationValidHonoured(t *testing.T) {
	t.Setenv("AUTO_COMPLETE_AFTER", "12s")
	t.Setenv("RETURN_WINDOW", "20s")
	t.Setenv("RETURN_AUTO_APPROVE_AFTER", "30s")
	t.Setenv("SHIP_BY_AFTER", "60s")
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("startup must not fail: %v", err)
	}
	if cfg.AutoCompleteAfter() != 12*time.Second || cfg.ReturnWindow() != 20*time.Second ||
		cfg.ReturnAutoApproveAfter() != 30*time.Second || cfg.ShipByAfter() != 60*time.Second {
		t.Errorf("values not honoured: %s %s %s %s", cfg.AutoCompleteAfter(), cfg.ReturnWindow(), cfg.ReturnAutoApproveAfter(), cfg.ShipByAfter())
	}
}

// Scenario: Order receipt: A malformed duration stops startup
func TestLifecycleDurationMalformedRejected(t *testing.T) {
	for _, k := range receiptDurationEnvs() {
		for _, bad := range []string{"abc", "7", "1 day"} {
			t.Run(k+"="+bad, func(t *testing.T) {
				setValidReceiptEnvs(t)
				t.Setenv(k, bad)
				_, err := config.LoadSettings()
				if err == nil {
					t.Fatalf("startup must fail for %s=%q", k, bad)
				}
				if !strings.Contains(err.Error(), k) {
					t.Errorf("error %q must name %s", err, k)
				}
			})
		}
	}
}

// Scenario: Order receipt: A zero or negative duration stops startup
func TestLifecycleDurationNonPositiveRejected(t *testing.T) {
	for _, k := range receiptDurationEnvs() {
		for _, bad := range []string{"0s", "0", "-1h"} {
			t.Run(k+"="+bad, func(t *testing.T) {
				setValidReceiptEnvs(t)
				t.Setenv(k, bad)
				_, err := config.LoadSettings()
				if err == nil {
					t.Fatalf("startup must fail for %s=%q", k, bad)
				}
				if !strings.Contains(err.Error(), k) {
					t.Errorf("error %q must name %s", err, k)
				}
			})
		}
	}
}
