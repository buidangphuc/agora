package config

import "testing"

func setBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("JWT_PRIVATE_KEY", "dev-rsa-private-key-pem")
	t.Setenv("JWT_KID", "dev-2026")
}

// Scenario: User enforcement: No audit upstream configured (default is empty = disabled).
func TestUpstreamAuditAddrDefaultEmpty_NoAuditUpstreamConfigured(t *testing.T) {
	setBase(t)
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.AuditAddr != "" {
		t.Errorf("default UPSTREAM_AUDIT_ADDR = %q, want empty", s.Upstream.AuditAddr)
	}
	if s.Upstream.CallTimeoutSeconds != 2 {
		t.Errorf("default UPSTREAM_CALL_TIMEOUT_SECONDS = %v, want 2", s.Upstream.CallTimeoutSeconds)
	}
}

// Scenario: User enforcement: Audit is called as the service principal (address is configurable).
func TestUpstreamAuditAddrFromEnv(t *testing.T) {
	setBase(t)
	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", "3.5")
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Upstream.AuditAddr != "team-audit-svc:50066" || s.Upstream.CallTimeoutSeconds != 3.5 {
		t.Errorf("Upstream = %+v", s.Upstream)
	}
}

// Scenario: User enforcement: Audit down refuses the change (a non-positive timeout is refused at startup).
func TestUpstreamCallTimeoutMustBePositive(t *testing.T) {
	for _, v := range []string{"0", "-1"} {
		setBase(t)
		t.Setenv("UPSTREAM_CALL_TIMEOUT_SECONDS", v)
		if _, err := LoadSettings(); err == nil {
			t.Errorf("UPSTREAM_CALL_TIMEOUT_SECONDS=%s must be rejected", v)
		}
	}
}
