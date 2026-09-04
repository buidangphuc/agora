package authz_test

import (
	"testing"

	"github.com/buidangphuc/team-identity/internal/authz"
)

func TestScopesForRoles(t *testing.T) {
	t.Run("Admin Scopes", func(t *testing.T) {
		scopes := authz.ScopesForRoles([]string{authz.RoleAdmin})
		if len(scopes) == 0 {
			t.Fatalf("expected non-empty admin scopes")
		}
	})

	t.Run("Buyer Scopes", func(t *testing.T) {
		scopes := authz.ScopesForRoles([]string{authz.RoleBuyer})
		if len(scopes) == 0 {
			t.Fatalf("expected non-empty buyer scopes")
		}
	})

	t.Run("Seller Scopes", func(t *testing.T) {
		scopes := authz.ScopesForRoles([]string{authz.RoleSeller})
		if len(scopes) == 0 {
			t.Fatalf("expected non-empty seller scopes")
		}
	})
}

func TestNormalizeRole(t *testing.T) {
	if authz.NormalizeRole("seller") != authz.RoleSeller {
		t.Errorf("expected RoleSeller")
	}
	if authz.NormalizeRole("buyer") != authz.RoleBuyer {
		t.Errorf("expected RoleBuyer")
	}
	if authz.NormalizeRole("admin") != authz.RoleBuyer {
		t.Errorf("expected admin self-assignment to fallback to RoleBuyer")
	}
	if authz.NormalizeRole("invalid") != authz.RoleBuyer {
		t.Errorf("expected fallback to RoleBuyer")
	}
}
