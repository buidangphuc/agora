package config

import (
	"os"
	"testing"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`).
func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50052 {
		t.Errorf("default GRPC_PORT = %d, want 50052", s.Server.Port)
	}
	if s.OpenSearch.Index != "listings" {
		t.Errorf("default OPENSEARCH_INDEX = %q, want listings", s.OpenSearch.Index)
	}
}

func TestValidateRequiresOpenSearchURL(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50052
	s.OpenSearch.URL = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: OPENSEARCH_URL required")
	}
}
