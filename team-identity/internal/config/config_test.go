package config

import (
	"os"
	"testing"
)

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
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "dev-rsa-private-key-pem")
	t.Setenv("JWT_KID", "dev-2026")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Server.Port != 50053 {
		t.Errorf("default GRPC_PORT = %d, want 50053", s.Server.Port)
	}
	if s.JWT.JWKSHTTPPort != 50063 {
		t.Errorf("default JWKS_HTTP_PORT = %d, want 50063", s.JWT.JWKSHTTPPort)
	}
	if s.JWT.TTLSeconds != 3600 {
		t.Errorf("default JWT_TTL_SECONDS = %d, want 3600", s.JWT.TTLSeconds)
	}
}

func TestValidateRequiresPrivateKeyAndKID(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50053
	s.JWT.JWKSHTTPPort = 50063
	s.JWT.TTLSeconds = 3600
	s.Database.Enabled = false
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: JWT_PRIVATE_KEY required")
	}
	s.JWT.PrivateKey = "pem"
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: JWT_KID required")
	}
	s.JWT.KID = "dev-2026"
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid settings, got %v", err)
	}
}
