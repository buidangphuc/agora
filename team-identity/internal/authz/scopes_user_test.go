package authz_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/buidangphuc/team-identity/internal/authz"
)

func sorted(s []string) []string {
	c := append([]string{}, s...)
	sort.Strings(c)
	return c
}

func without(s []string, drop string) []string {
	var out []string
	for _, x := range s {
		if x != drop {
			out = append(out, x)
		}
	}
	return out
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Scenario: Login enforcement: Restricted seller token lacks listing.write
func TestScopesForUserRestricted(t *testing.T) {
	roles := []string{authz.RoleSeller}
	got := authz.ScopesForUser(roles, "restricted")
	if has(got, "listing.write") {
		t.Fatalf("restricted seller must not hold listing.write: %v", got)
	}
	for _, s := range []string{"listing.read", "engagement:write", "search:write"} {
		if !has(got, s) {
			t.Errorf("restricted seller must keep %s: %v", s, got)
		}
	}
	want := without(authz.ScopesForRoles(roles), "listing.write")
	if !reflect.DeepEqual(sorted(got), sorted(want)) {
		t.Fatalf("scopes = %v, want seller scopes minus listing.write = %v", got, want)
	}
}

// Scenario: Login enforcement: Active user scopes unchanged
func TestScopesForUserActiveUnchanged(t *testing.T) {
	for _, roles := range [][]string{{authz.RoleSeller}, {authz.RoleAdmin}, {authz.RoleBuyer}, {authz.RoleSeller, authz.RoleBuyer}} {
		got := authz.ScopesForUser(roles, "active")
		if !reflect.DeepEqual(got, authz.ScopesForRoles(roles)) {
			t.Errorf("roles %v active: got %v, want %v", roles, got, authz.ScopesForRoles(roles))
		}
	}
	if !has(authz.ScopesForUser([]string{authz.RoleAdmin}, "active"), "listing.write") {
		t.Fatal("active admin must keep listing.write")
	}
}

// Scenario: Login enforcement: Unknown status gets no extra scopes
func TestScopesForUserUnknownStatusFailsClosed(t *testing.T) {
	roles := []string{authz.RoleSeller}
	for _, st := range []string{"weird", "", "ACTIVE", "Active", " active"} {
		got := authz.ScopesForUser(roles, st)
		want := without(authz.ScopesForRoles(roles), "listing.write")
		if !reflect.DeepEqual(sorted(got), sorted(want)) {
			t.Errorf("status %q: got %v, want %v", st, got, want)
		}
	}
}

// Scenario: Login enforcement: Restricted seller token lacks listing.write
// (ScopesForUser must not mutate the shared role table.)
func TestScopesForUserDoesNotMutateRoleTable(t *testing.T) {
	_ = authz.ScopesForUser([]string{authz.RoleSeller}, "restricted")
	if !has(authz.ScopesForRoles([]string{authz.RoleSeller}), "listing.write") {
		t.Fatal("restricted call corrupted the seller role table")
	}
}

// Scenario: Login enforcement: Restricted seller token lacks listing.write
// (a restricted admin is never produced, but admin scopes besides listing.write stay.)
func TestScopesForUserRestrictedKeepsAdminMarker(t *testing.T) {
	got := authz.ScopesForUser([]string{authz.RoleAdmin}, "restricted")
	if !has(got, "admin") || has(got, "listing.write") {
		t.Fatalf("unexpected restricted admin scopes: %v", got)
	}
}

// Task 5: AccountAdminService RPCs are admin-scope, never public.
func TestAccountAdminRPCsAreNotPublicAndNeedAdminScope(t *testing.T) {
	for _, m := range []string{"ListUsers", "SetUserStatus"} {
		if authz.IsPublicRPC("/platform.identity.v1.AccountAdminService/" + m) {
			t.Errorf("AccountAdminService/%s must not be public", m)
		}
	}
	for _, role := range []string{authz.RoleBuyer, authz.RoleSeller} {
		if has(authz.ScopesForRoles([]string{role}), "admin") {
			t.Errorf("role %s must not hold the admin scope", role)
		}
	}
	if !has(authz.ScopesForRoles([]string{authz.RoleAdmin}), "admin") {
		t.Error("admin role must hold the admin scope")
	}
}
