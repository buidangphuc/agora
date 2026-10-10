package authz_test

import (
	"sort"
	"testing"

	"github.com/buidangphuc/team-identity/internal/authz"
)

// enforcedScope is a scope some service checks (RequireScopes / ensure_scopes) on behalf of
// a signed-in user. Keep this list in step with the services: a new scope gate added in any
// service must be declared here, and this test then proves identity can actually issue it.
type enforcedScope struct {
	scope   string
	service string
}

var enforcedScopes = []enforcedScope{
	{"listing.read", "team-domain"},
	{"listing.write", "team-domain, team-payment (seller-bound wallet/payout/refund), team-analytics, team-promotion (ad campaigns)"},
	{"search:read", "team-search"},
	{"search:write", "team-search"},
	{"engagement:read", "team-engagement"},
	{"engagement:write", "team-engagement"},
	{"recommendations:read", "team-ai"},
	{"ai:use", "team-ai"},
	{"order.admin", "team-order (ForceFailSaga with admin; admin override on GetOrder, GetSagaState, UpdateOrderStatus, shipment and return RPCs)"},
	{"admin", "team-gateway edge policy, team-engagement, team-verification, team-audit, team-promotion, team-payment, team-analytics, team-domain (admin-only RPCs and ownership overrides)"},
}

// serviceOnlyScopes are enforced by a service but deliberately granted to no user role;
// they live only on a service principal (design D9).
var serviceOnlyScopes = []enforcedScope{
	{"inventory.write", "reserved for team-domain stock RPCs; today they gate on SERVICE + listing.write (authz-hardening-wave2)"},
	{"order.read", "team-order (GetOrder for a service principal, held by team-payment and team-engagement)"},
	{"promotion.reserve", "team-promotion (ValidateAndReserve service path, CommitReservation, ReleaseReservation; held by service-team-order)"},
	{"audit.write", "team-audit (WriteAuditEvent; held by any audit-producing service)"},
	{"identity.read", "team-identity (GetPublicProfiles for a service principal, held by team-notification)"},
	{"features.read", "team-analytics FeatureService GetOnlineFeatures/DescribeFeatures (held by service-team-ai)"},
	{"features.dataset", "team-analytics FeatureService BuildDataset/GetDatasetBuild (held by service-platform-recsys, the recsys trainer)"},
}

// serviceOnlyHeld returns every (role, scope) pair in the given role table that grants a
// service-only scope; a correct table yields none.
func serviceOnlyHeld(table map[string][]string) []string {
	var held []string
	for role, scopes := range table {
		for _, s := range scopes {
			for _, so := range serviceOnlyScopes {
				if s == so.scope {
					held = append(held, role+":"+s)
				}
			}
		}
	}
	return held
}

// A user role holding features.read or features.dataset must be detected.
func TestServiceOnlyGuardCatchesFeatureScopes(t *testing.T) {
	for _, sc := range []string{"features.read", "features.dataset"} {
		got := serviceOnlyHeld(map[string][]string{"buyer": {"listing.read", sc}})
		if len(got) != 1 || got[0] != "buyer:"+sc {
			t.Fatalf("guard missed %s granted to a role: %v", sc, got)
		}
	}
	if got := serviceOnlyHeld(map[string][]string{"buyer": {"listing.read"}}); len(got) != 0 {
		t.Fatalf("false positive: %v", got)
	}
}

// uncoveredScopes returns every declared scope that none of the roles is granted.
func uncoveredScopes(declared []enforcedScope, roles []string) []string {
	granted := map[string]bool{}
	for _, s := range authz.ScopesForRoles(roles) {
		granted[s] = true
	}
	var missing []string
	for _, e := range declared {
		if !granted[e.scope] {
			missing = append(missing, e.scope+" (enforced by "+e.service+")")
		}
	}
	sort.Strings(missing)
	return missing
}

func TestEveryEnforcedScopeIsGrantedToSomeRole(t *testing.T) {
	roles := []string{authz.RoleBuyer, authz.RoleSeller, authz.RoleAdmin}
	if missing := uncoveredScopes(enforcedScopes, roles); len(missing) > 0 {
		t.Fatalf("scopes enforced by a service but granted to no role: %v", missing)
	}
}

func TestCoverageCheckNamesAnUngrantedScope(t *testing.T) {
	declared := append(append([]enforcedScope{}, enforcedScopes...), enforcedScope{"ghost:write", "team-ghost"})
	missing := uncoveredScopes(declared, []string{authz.RoleBuyer, authz.RoleSeller, authz.RoleAdmin})
	if len(missing) != 1 || missing[0] != "ghost:write (enforced by team-ghost)" {
		t.Fatalf("expected the ungranted scope to be named, got %v", missing)
	}
}

func TestServiceOnlyScopesGrantedToNoRole(t *testing.T) {
	for _, role := range []string{authz.RoleBuyer, authz.RoleSeller, authz.RoleAdmin} {
		for _, s := range authz.ScopesForRoles([]string{role}) {
			for _, so := range serviceOnlyScopes {
				if s == so.scope {
					t.Errorf("role %s must not be granted service-only scope %s (%s)", role, s, so.service)
				}
			}
		}
	}
	all := authz.ScopesForRoles([]string{authz.RoleBuyer, authz.RoleSeller, authz.RoleAdmin})
	for _, s := range all {
		for _, so := range serviceOnlyScopes {
			if s == so.scope {
				t.Fatalf("service-only scope %s is granted by the role table", s)
			}
		}
	}
}
