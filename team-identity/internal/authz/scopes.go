// Package authz maps roles to scopes. This is deliberately a small in-code table
// (basic RBAC); it is the seam to grow into a real permission model later.
package authz

// Known roles.
const (
	RoleAdmin  = "admin"
	RoleSeller = "seller"
	RoleBuyer  = "buyer"
)

// roleScopes is the role → scopes table. Scopes match what services enforce via
// RequireScopes (listing.read/write, search:read/write, engagement:read/write,
// recommendations:read, ai:use) plus the `admin` marker, granted to the admin role only.
// recommendations:read and ai:use are the team-ai gates (gateway-and-ai-hardening D7,
// order-domain-correctness); they are not in the gateway's public (anonymous) scope set.
//
// Seven SERVICE-ONLY scopes are deliberately granted to NO role; they live only on a
// service principal and are declared in coverage_test.go (serviceOnlyScopes):
//   - inventory.write   reserved for team-domain stock RPCs (not enforced yet: they gate on
//     a SERVICE principal with listing.write; authz-hardening-wave2 moves them here)
//   - order.read        team-order GetOrder, held by the service principals of team-payment
//     and team-engagement
//   - promotion.reserve team-promotion voucher saga RPCs, held by service-team-order
//   - audit.write       team-audit WriteAuditEvent, held by audit-producing services
//   - identity.read     team-identity GetPublicProfiles, held by team-notification
//   - features.read     FeatureService online serving, held by service-team-ai
//   - features.dataset  FeatureService BuildDataset/GetDatasetBuild, held by the recsys trainer
//
// See TestServiceOnlyScopesGrantedToNoRole.
var roleScopes = map[string][]string{
	RoleAdmin:  {"listing.read", "listing.write", "search:read", "search:write", "engagement:read", "engagement:write", "recommendations:read", "ai:use", "admin"},
	RoleSeller: {"listing.read", "listing.write", "search:read", "search:write", "engagement:read", "engagement:write", "recommendations:read", "ai:use"},
	RoleBuyer:  {"listing.read", "search:read", "search:write", "engagement:read", "engagement:write", "recommendations:read", "ai:use"},
}

// ScopesForRoles returns the deduped union of scopes granted by the given roles.
func ScopesForRoles(roles []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range roles {
		for _, s := range roleScopes[r] {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// NormalizeRole validates a self-assignable role at registration; unknown or
// admin (seeded only) falls back to buyer.
func NormalizeRole(role string) string {
	switch role {
	case RoleSeller:
		return RoleSeller
	case RoleBuyer:
		return RoleBuyer
	default:
		return RoleBuyer
	}
}
