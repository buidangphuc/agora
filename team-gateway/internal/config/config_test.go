package config_test

import (
	"os"
	"strings"
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

func validSettings() *config.Settings {
	s := &config.Settings{}
	s.Server.Port = 8080
	s.Upstream.SearchAddr = "localhost:50052"
	s.Upstream.ListingAddr = "localhost:50051"
	s.Upstream.IdentityAddr = "localhost:50053"
	s.Auth.JWKSURL = "http://identity/.well-known/jwks.json"
	s.Auth.JWKSCacheTTLSeconds = 300
	return s
}

func TestValidateRefusesReflectionInStrictEnv(t *testing.T) {
	for _, env := range []string{"staging", "production", "prod", "Production"} {
		s := validSettings()
		s.Runtime.Env = env
		s.Edge.ReflectionEnabled = true
		err := s.Validate()
		if err == nil || !strings.Contains(err.Error(), "EDGE_REFLECTION_ENABLED") {
			t.Errorf("ENV=%s: want an error naming EDGE_REFLECTION_ENABLED, got %v", env, err)
		}
		s.Edge.ReflectionEnabled = false
		if err := s.Validate(); err != nil {
			t.Errorf("ENV=%s with reflection off: unexpected %v", env, err)
		}
	}
	for _, env := range []string{"local", "test", ""} {
		s := validSettings()
		s.Runtime.Env = env
		s.Edge.ReflectionEnabled = true
		if err := s.Validate(); err != nil {
			t.Errorf("ENV=%q must allow reflection: %v", env, err)
		}
	}
}

func TestEdgeDefaults(t *testing.T) {
	t.Setenv("JWKS_URL", "http://identity/jwks")
	cfg, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Edge.ReflectionEnabled {
		t.Error("EDGE_REFLECTION_ENABLED must default to false")
	}
	if cfg.Upstream.DialTimeout != 2 || cfg.Edge.AICallTimeoutSecs != 30 || cfg.Edge.StreamMaxRequestBytes != 16384 ||
		cfg.Edge.TrackRateLimitRPS != 5 || cfg.Edge.TrackRateLimitBurst != 20 {
		t.Errorf("unexpected defaults: %+v %+v", cfg.Upstream.DialTimeout, cfg.Edge)
	}
}

// TestEnvExampleInSync is the merge gate behind `make check-env`: every Settings
// env key is documented in .env.example and nothing stale is left there.
func TestEnvExampleInSync(t *testing.T) {
	if err := config.CheckEnvExample("../../.env.example"); err != nil {
		t.Fatal(err)
	}
}
