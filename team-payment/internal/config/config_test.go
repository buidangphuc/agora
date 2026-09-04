package config_test

import (
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
