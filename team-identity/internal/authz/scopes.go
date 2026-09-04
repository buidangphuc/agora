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
// RequireScopes (listing.read/write, search:read) plus an admin marker.
var roleScopes = map[string][]string{
	RoleAdmin:  {"listing.read", "listing.write", "search:read", "search:write", "engagement:read", "engagement:write", "admin"},
	RoleSeller: {"listing.read", "listing.write", "search:read", "search:write", "engagement:read", "engagement:write"},
	RoleBuyer:  {"listing.read", "search:read", "search:write", "engagement:read", "engagement:write"},
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
