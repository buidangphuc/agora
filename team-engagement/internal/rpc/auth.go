package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

// The SDK's auth interceptor (installed by grpcx.NewServer) resolves the
// gateway-forwarded principal into the context (ADR-0003). These helpers are this
// service's own gates over it. They stay local: auth.RequireScopes treats the
// anonymous principal as unauthenticated, while the gateway gives anonymous
// callers the public scopes (listing.read) so that public reads work, and
// auth.RequireScopesAllowAnonymous words its errors differently ("missing scope",
// "authentication required") from the messages clients get today
// ("insufficient_scope: missing ...", "no principal on context").

const (
	// scopeAdmin gates admin-only RPCs (ResolveDispute). Only the admin role holds it.
	scopeAdmin = "admin"
)

// requireScopes returns codes.PermissionDenied with an `insufficient_scope`
// message unless the context principal holds every scope in want, and
// codes.Unauthenticated when the call carries no principal.
func requireScopes(ctx context.Context, want ...string) error {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no principal on context")
	}
	for _, s := range want {
		if !p.HasScope(s) {
			return status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", s)
		}
	}
	return nil
}

// requireAuthenticated returns the caller's principal, or UNAUTHENTICATED when
// there is none, it is anonymous, or it has no id.
func requireAuthenticated(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.FromContext(ctx)
	if !ok || p.ID == "" || p.Type == auth.TypeAnonymous {
		return auth.Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

// requireAdmin requires an authenticated caller holding the `admin` scope:
// UNAUTHENTICATED for none/anonymous, PERMISSION_DENIED otherwise.
func requireAdmin(ctx context.Context) error {
	if _, err := requireAuthenticated(ctx); err != nil {
		return err
	}
	return requireScopes(ctx, scopeAdmin)
}

// isAdmin reports whether the caller is authenticated and holds `admin`.
func isAdmin(ctx context.Context) bool {
	return requireAdmin(ctx) == nil
}
