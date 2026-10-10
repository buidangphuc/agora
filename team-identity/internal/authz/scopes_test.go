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

func TestUserRolesCarryRecommendationsAndAI(t *testing.T) {
	for _, role := range []string{authz.RoleBuyer, authz.RoleSeller, authz.RoleAdmin} {
		got := map[string]bool{}
		for _, s := range authz.ScopesForRoles([]string{role}) {
			got[s] = true
		}
		for _, want := range []string{"recommendations:read", "ai:use"} {
			if !got[want] {
				t.Errorf("role %s missing scope %s", role, want)
			}
		}
	}
}

// order.admin is a role scope held by the admin role and no other (authz-residuals-2).
func TestOrderAdminIsAdminOnly(t *testing.T) {
	has := func(role string) bool {
		for _, s := range authz.ScopesForRoles([]string{role}) {
			if s == "order.admin" {
				return true
			}
		}
		return false
	}
	if !has(authz.RoleAdmin) {
		t.Fatal("admin role must be issued order.admin")
	}
	for _, r := range []string{authz.RoleSeller, authz.RoleBuyer} {
		if has(r) {
			t.Fatalf("%s role must not be issued order.admin", r)
		}
	}
}
