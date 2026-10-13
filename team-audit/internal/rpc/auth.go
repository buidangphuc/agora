package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

const (
	// scopeAdmin is held only by the admin role (team-identity/internal/authz).
	scopeAdmin = "admin"
	// scopeAuditWrite is the scope a service principal must hold to append audit events.
	scopeAuditWrite = "audit.write"
)

// isService reports whether p is a service (not a user) principal.
func isService(p auth.Principal) bool {
	return !p.Anonymous() && p.Type == auth.TypeService
}

// requireAdmin authenticates the caller and requires the admin scope:
// Unauthenticated with no or an anonymous principal, PermissionDenied without
// the scope (a service principal needs it too).
func requireAdmin(ctx context.Context) (auth.Principal, error) {
	return auth.RequireScopes(ctx, scopeAdmin)
}

// requireService authenticates the caller and requires a principal of type
// service holding scope. Users (admins included), anonymous callers and
// scopeless services are refused: Unauthenticated when there is no usable
// principal, PermissionDenied otherwise. The SDK has no service-type check, so
// this is a local helper on top of auth.RequirePrincipal and auth.RequireScopes.
func requireService(ctx context.Context, scope string) (auth.Principal, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if !isService(p) {
		return auth.Principal{}, status.Error(codes.PermissionDenied, "service principal required")
	}
	return auth.RequireScopes(ctx, scope)
}
