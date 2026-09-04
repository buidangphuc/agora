package config

import (
	"os"
	"testing"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`): the repo-root
// .env.example must document exactly the env keys Settings declares.
func TestEnvExampleInSync(t *testing.T) {
	// Test runs with working dir = this package; .env.example is at the repo root.
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	// DATABASE_URL is required because DATABASE_ENABLED defaults to true; set it
	// so Validate passes, then assert the other struct defaults apply.
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50051 {
		t.Errorf("default GRPC_PORT = %d, want 50051", s.Server.Port)
	}
	if !s.Database.Enabled {
		t.Error("default DATABASE_ENABLED should be true")
	}
	if got := s.KafkaBrokers(); len(got) != 1 || got[0] != "localhost:9092" {
		t.Errorf("default KafkaBrokers = %v, want [localhost:9092]", got)
	}
}

func TestValidateDatabaseURLRequired(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50051
	s.Database.Enabled = true
	s.Database.URL = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: DATABASE_ENABLED=true requires DATABASE_URL")
	}
}
