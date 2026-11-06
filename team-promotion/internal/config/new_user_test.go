package config_test

import (
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/config"
)

func asDuration(t *testing.T, v any) time.Duration {
	t.Helper()
	d, ok := v.(time.Duration)
	if !ok {
		t.Fatalf("setting is %T, want time.Duration", v)
	}
	return d
}

func TestNewUserConfigDefaults(t *testing.T) {
	localEnv(t)
	s, err := loadSettings(t)
	if err != nil {
		t.Fatalf("defaults must boot: %v", err)
	}
	for key, want := range map[string]int64{
		"NEW_USER_WELCOME_AMOUNT": 20000,
		"NEW_USER_INVITED_AMOUNT": 30000,
		"NEW_USER_MIN_SPEND":      100000,
	} {
		if got := asInt64(t, settingByEnvKey(t, s, key)); got != want {
			t.Errorf("%s default = %d, want %d", key, got, want)
		}
	}
	for key, want := range map[string]time.Duration{
		"NEW_USER_VOUCHER_TTL":            336 * time.Hour,
		"NEW_USER_VERDICT_TIMEOUT":        30 * time.Minute,
		"NEW_USER_VERDICT_SWEEP_INTERVAL": time.Minute,
		"VOUCHER_RESERVATION_TTL":         15 * time.Minute,
		"UPSTREAM_ORDER_TIMEOUT":          500 * time.Millisecond,
	} {
		if got := asDuration(t, settingByEnvKey(t, s, key)); got != want {
			t.Errorf("%s default = %v, want %v", key, got, want)
		}
	}
}

func TestNewUserConfigValidate(t *testing.T) {
	for _, c := range []struct{ key, val string }{
		{"NEW_USER_WELCOME_AMOUNT", "0"},
		{"NEW_USER_WELCOME_AMOUNT", "-1"},
		{"NEW_USER_WELCOME_AMOUNT", "abc"},
		{"NEW_USER_INVITED_AMOUNT", "0"},
		{"NEW_USER_INVITED_AMOUNT", "-30000"},
		{"NEW_USER_MIN_SPEND", "-1"},
		{"NEW_USER_VOUCHER_TTL", "0"},
		{"NEW_USER_VOUCHER_TTL", "0s"},
		{"NEW_USER_VOUCHER_TTL", "-1h"},
		{"NEW_USER_VERDICT_TIMEOUT", "0"},
		{"NEW_USER_VERDICT_TIMEOUT", "0s"},
		{"NEW_USER_VERDICT_SWEEP_INTERVAL", "0"},
		{"VOUCHER_RESERVATION_TTL", "0"},
		{"VOUCHER_RESERVATION_TTL", "0s"},
		{"VOUCHER_RESERVATION_TTL", "-15m"},
	} {
		t.Run(c.key+"="+c.val, func(t *testing.T) {
			localEnv(t)
			t.Setenv(c.key, c.val)
			_, err := loadSettings(t)
			mustErrNaming(t, err, c.key)
		})
	}

	localEnv(t)
	t.Setenv("NEW_USER_WELCOME_AMOUNT", "1")
	t.Setenv("NEW_USER_INVITED_AMOUNT", "1")
	t.Setenv("NEW_USER_MIN_SPEND", "0")
	t.Setenv("NEW_USER_VOUCHER_TTL", "1s")
	t.Setenv("NEW_USER_VERDICT_TIMEOUT", "2s")
	t.Setenv("NEW_USER_VERDICT_SWEEP_INTERVAL", "1s")
	t.Setenv("VOUCHER_RESERVATION_TTL", "1s")
	s, err := loadSettings(t)
	if err != nil {
		t.Fatalf("smallest valid values must boot: %v", err)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "NEW_USER_MIN_SPEND")); got != 0 {
		t.Errorf("NEW_USER_MIN_SPEND = %d, want 0 accepted", got)
	}
	if got := asDuration(t, settingByEnvKey(t, s, "NEW_USER_VERDICT_TIMEOUT")); got != 2*time.Second {
		t.Errorf("NEW_USER_VERDICT_TIMEOUT = %v, want 2s", got)
	}
}

func TestOrderAddrIsOptionalAtBoot(t *testing.T) {
	localEnv(t)
	if _, err := loadSettings(t); err != nil {
		t.Fatalf("an unset UPSTREAM_ORDER_ADDR must not stop boot (checks fail closed at call time): %v", err)
	}
	t.Setenv("UPSTREAM_ORDER_ADDR", "team-order-svc:50055")
	s, err := loadSettings(t)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := settingByEnvKey(t, s, "UPSTREAM_ORDER_ADDR").(string); got != "team-order-svc:50055" {
		t.Errorf("UPSTREAM_ORDER_ADDR = %q", got)
	}
}

func loadSettings(t *testing.T) (config.Settings, error) {
	t.Helper()
	return config.Load()
}
