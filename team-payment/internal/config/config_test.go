package config_test

import (
	"strings"
	"testing"

	"github.com/buidangphuc/team-payment/internal/config"
)

func TestLoadSettings_Defaults(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("GRPC_PORT", "50056")

	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if s.Server.Port != 50056 {
		t.Errorf("expected GRPC_PORT 50056, got %d", s.Server.Port)
	}
	if s.Database.Enabled {
		t.Errorf("expected Database.Enabled to be false")
	}
	if s.IsProd() {
		t.Errorf("expected IsProd to be false for local env")
	}
}

func TestLoadSettings_ValidationErrors(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "")

	_, err := config.LoadSettings()
	if err == nil {
		t.Fatal("expected error when DATABASE_ENABLED=true and DATABASE_URL is empty")
	}

	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("GRPC_PORT", "999999")
	_, err = config.LoadSettings()
	if err == nil {
		t.Fatal("expected error when GRPC_PORT is out of range")
	}
}

func TestDeclaredEnvKeys(t *testing.T) {
	keys := config.DeclaredEnvKeys()
	if len(keys) == 0 {
		t.Fatal("expected non-empty declared env keys")
	}
}

func TestRequireNoMockPayments(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "false")
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
			s, err := config.LoadSettings()
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
