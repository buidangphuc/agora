package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-gateway/internal/config"
)

func TestGatewayConfig(t *testing.T) {
	os.Setenv("JWKS_URL", "http://team-identity:50063/.well-known/jwks.json")
	os.Setenv("HTTP_PORT", "9090")
	defer func() {
		os.Unsetenv("JWKS_URL")
		os.Unsetenv("HTTP_PORT")
	}()

	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Auth.JWKSURL != "http://team-identity:50063/.well-known/jwks.json" {
		t.Errorf("expected JWKS_URL, got %s", cfg.Auth.JWKSURL)
	}
	if cfg.Auth.JWKSCacheTTLSeconds != 300 {
		t.Errorf("expected default JWKS_CACHE_TTL 300, got %d", cfg.Auth.JWKSCacheTTLSeconds)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
}

func TestValidateRequiresJWKSURL(t *testing.T) {
	s := &config.Settings{}
	s.Server.Port = 8080
	s.Upstream.SearchAddr = "localhost:50052"
	s.Upstream.ListingAddr = "localhost:50051"
	s.Upstream.IdentityAddr = "localhost:50053"
	s.Auth.JWKSCacheTTLSeconds = 300
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: JWKS_URL required")
	}
}
