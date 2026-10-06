package config

import (
	"os"
	"strings"
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

func seedSettings(enabled bool, user, pw string) *Settings {
	s := &Settings{}
	s.Server.Port = 50053
	s.JWT.JWKSHTTPPort = 50063
	s.JWT.TTLSeconds = 3600
	s.JWT.PrivateKey = "pem"
	s.JWT.KID = "k"
	s.SeedAdmin = SeedAdmin{Enabled: enabled, Username: user, Password: pw}
	return s
}

func TestSeedAdminValidation(t *testing.T) {
	cases := []struct {
		name    string
		s       *Settings
		wantErr string
	}{
		{"disabled ignores password", seedSettings(false, "admin", ""), ""},
		{"enabled without password", seedSettings(true, "admin", ""), "SEED_ADMIN_PASSWORD"},
		{"enabled short password", seedSettings(true, "admin", "elevenchars"), "SEED_ADMIN_PASSWORD"},
		{"enabled empty username", seedSettings(true, " ", "a-long-enough-password"), "SEED_ADMIN_USERNAME"},
		{"enabled valid", seedSettings(true, "admin", "twelve-chars"), ""},
	}
	for _, c := range cases {
		err := c.s.Validate()
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: err = %v, want mention of %s", c.name, err, c.wantErr)
		}
	}
}

func TestSeedAdminDefaultsOff(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "pem")
	t.Setenv("JWT_KID", "k")
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.SeedAdmin.Enabled || s.SeedAdmin.Password != "" || s.SeedAdmin.Username != "admin" {
		t.Errorf("seed defaults = %+v, want disabled, empty password, username admin", s.SeedAdmin)
	}
}

func TestSeedAdminLoadFailsFast(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "pem")
	t.Setenv("JWT_KID", "k")
	t.Setenv("SEED_ADMIN_ENABLED", "true")
	if _, err := LoadSettings(); err == nil || !strings.Contains(err.Error(), "SEED_ADMIN_PASSWORD") {
		t.Fatalf("LoadSettings err = %v, want SEED_ADMIN_PASSWORD error", err)
	}
}

func TestPasswordResetExposeTokenDefaultsOffAndRefusedInProd(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "pem")
	t.Setenv("JWT_KID", "k")
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.PasswordReset.ExposeToken {
		t.Fatal("PASSWORD_RESET_EXPOSE_TOKEN must default to false")
	}
	t.Setenv("PASSWORD_RESET_EXPOSE_TOKEN", "true")
	if _, err := LoadSettings(); err != nil {
		t.Fatalf("local env should allow it: %v", err)
	}
	t.Setenv("ENV", "prod")
	if _, err := LoadSettings(); err == nil || !strings.Contains(err.Error(), "PASSWORD_RESET_EXPOSE_TOKEN") {
		t.Fatalf("prod must refuse it, got %v", err)
	}
}
