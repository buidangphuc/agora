package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

const (
	// scopeAdmin marks platform administrators (granted by team-identity to the admin role).
	scopeAdmin = "admin"
	// scopeListingWrite is granted to seller and admin roles, never to buyers.
	scopeListingWrite = "listing.write"
	// scopePromoReserve is service-only (held by service-team-order, granted to no
	// user role): it gates the voucher redemption saga RPCs.
	scopePromoReserve = "promotion.reserve"
)

// isService reports whether p is a service principal.
func isService(p auth.Principal) bool { return p.Type == auth.TypeService }

// requireService demands a principal of type service that also holds scope. The
// type is checked, not only the scope string: a user principal that carries the
// scope is PERMISSION_DENIED. It stays local because auth.RequireService checks
// the type first and answers "principal type not allowed" (or "missing scope"),
// where promotion has always answered "service principal required" after the
// scope check; the text is client-visible.
func requireService(ctx context.Context, scope string) (auth.Principal, error) {
	p, err := auth.RequireScopes(ctx, scope)
	if err != nil {
		return auth.Principal{}, err
	}
	if !isService(p) {
		return auth.Principal{}, status.Error(codes.PermissionDenied, "service principal required")
	}
	return p, nil
}

// isAdmin reports whether p carries the admin scope. Promotion decides what
// admin may do; the SDK treats the scope like any other.
func isAdmin(p auth.Principal) bool { return p.HasScope(scopeAdmin) }

// requireSeller allows seller and admin principals (listing.write or admin scope).
func requireSeller(p auth.Principal) error {
	if isAdmin(p) || p.HasScope(scopeListingWrite) {
		return nil
	}
	return status.Error(codes.PermissionDenied, "seller or admin role required")
}

// requireAdmin allows only admin principals.
func requireAdmin(p auth.Principal) error {
	if isAdmin(p) {
		return nil
	}
	return status.Error(codes.PermissionDenied, "admin role required")
}
