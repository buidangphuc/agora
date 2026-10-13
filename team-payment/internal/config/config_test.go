package config_test

import (
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-payment/internal/config"
)

// baseEnv sets the keys Load requires: the database URL and the gRPC port.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("GRPC_PORT", "50056")
}

func TestLoadSettings_Defaults(t *testing.T) {
	baseEnv(t)
	t.Setenv("GRPC_PORT", "50056")

	s, err := config.Load()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if s.Port != 50056 {
		t.Errorf("expected GRPC_PORT 50056, got %d", s.Port)
	}
	if s.IsProd() {
		t.Errorf("expected IsProd to be false for local env")
	}
}

func TestLoadSettings_ValidationErrors(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_ENABLED=true and DATABASE_URL is empty")
	}

	baseEnv(t)
	t.Setenv("GRPC_PORT", "999999")
	_, err = config.Load()
	if err == nil {
		t.Fatal("expected error when GRPC_PORT is out of range")
	}
}

func TestDeclaredEnvKeys(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if len(keys) == 0 {
		t.Fatal("expected non-empty declared env keys")
	}
}

func TestRequireNoMockPayments(t *testing.T) {
	baseEnv(t)
	cases := []struct {
		name, env, mock string
		wantErr         bool
	}{
		{"default off, unset env", "", "", false},
		{"default off, production", "production", "", false},
		{"true, local", "local", "true", false},
		{"true, staging", "staging", "true", true},
		{"true, stage", "stage", "true", true},
		{"true, prod", "prod", "true", true},
		{"true, production mixed case", " Production ", "true", true},
		{"false, production", "production", "false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("ENV", tc.env)
			}
			if tc.mock != "" {
				t.Setenv("MOCK_PAYMENTS", tc.mock)
			}
			s, err := config.Load()
			if err != nil {
				t.Fatalf("LoadSettings: %v", err)
			}
			if tc.mock == "" && s.Mock.MockPayments {
				t.Fatal("MOCK_PAYMENTS must default to false")
			}
			err = s.RequireNoMockPayments()
			if (err != nil) != tc.wantErr {
				t.Fatalf("RequireNoMockPayments() = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "MOCK_PAYMENTS") {
				t.Fatalf("error must name MOCK_PAYMENTS: %v", err)
			}
		})
	}
}

func TestPayoutHoldWindow(t *testing.T) {
	cases := []struct {
		name, days, window string
		want               time.Duration
		errKey             string // non-empty: startup must fail naming this key
	}{
		{name: "unset is 7 days", want: 7 * 24 * time.Hour},
		{name: "days", days: "3", want: 3 * 24 * time.Hour},
		{name: "0 disables", days: "0", want: 0},
		{name: "max days", days: "3650", want: 3650 * 24 * time.Hour},
		{name: "negative days", days: "-1", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "too many days", days: "3651", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "non-numeric days", days: "abc", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "window overrides days", days: "7", window: "20s", want: 20 * time.Second},
		{name: "window 0 disables", window: "0s", want: 0},
		{name: "unparsable window", window: "soon", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "negative window", window: "-1s", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "window above 3650 days", window: "87601h", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "invalid days still refused under a valid window", days: "-1", window: "20s", errKey: "PAYOUT_HOLD_DAYS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseEnv(t)
			if tc.days != "" {
				t.Setenv("PAYOUT_HOLD_DAYS", tc.days)
			}
			if tc.window != "" {
				t.Setenv("PAYOUT_HOLD_WINDOW", tc.window)
			}
			s, err := config.Load()
			if tc.errKey != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errKey) {
					t.Fatalf("want an error naming %s, got %v", tc.errKey, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSettings: %v", err)
			}
			if s.Payout.Window != tc.want {
				t.Fatalf("window = %s, want %s", s.Payout.Window, tc.want)
			}
		})
	}
}

func TestValidateNamesEveryInvalidSetting(t *testing.T) {
	cases := []struct {
		name string
		s    config.Settings
		keys []string
	}{
		{
			name: "invalid hold window without a database url",
			s:    config.Settings{GRPC: sdkconfig.GRPC{Port: 50056}, Payout: config.Payout{HoldDays: 7, HoldWindow: "soon"}},
			keys: []string{"DATABASE_URL", "PAYOUT_HOLD_WINDOW"},
		},
		{
			name: "mock payments in production without a database url",
			s: config.Settings{
				Runtime: sdkconfig.Runtime{Env: "production"},
				GRPC:    sdkconfig.GRPC{Port: 50056},
				Mock:    config.Mock{MockPayments: true},
			},
			keys: []string{"DATABASE_URL", "MOCK_PAYMENTS"},
		},
		{
			name: "bad port, hold days and hold window together",
			s: config.Settings{
				GRPC:     sdkconfig.GRPC{Port: 999999},
				Postgres: sdkconfig.Postgres{URL: "postgres://u:p@localhost/db"},
				Payout:   config.Payout{HoldDays: -1, HoldWindow: "soon"},
			},
			keys: []string{"GRPC_PORT", "PAYOUT_HOLD_DAYS"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.Validate()
			if err == nil {
				t.Fatal("expected Validate to fail")
			}
			for _, k := range tc.keys {
				if !strings.Contains(err.Error(), k) {
					t.Errorf("error must name %s: %v", k, err)
				}
			}
		})
	}
}
