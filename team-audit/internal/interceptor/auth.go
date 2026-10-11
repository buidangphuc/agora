// Package interceptor holds the gRPC auth/identity plumbing for team-audit.
//
// The gateway verifies the caller's JWT once and forwards the resulting
// principal as x-principal-{id,type,scopes} metadata (ADR-0003); this package
// reads exactly that, like every other service. The interceptor never rejects:
// each handler authorizes as its first statement.
package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Forwarded principal metadata keys (see team-gateway/internal/edge/forward.go).
const (
	mdPrincipalID     = "x-principal-id"
	mdPrincipalType   = "x-principal-type"
	mdPrincipalScopes = "x-principal-scopes"

	anonymousID   = "anonymous"
	typeAnonymous = "anonymous"
	typeService   = "service"

	// ScopeAdmin is held only by the admin role (team-identity/internal/authz).
	ScopeAdmin = "admin"
	// ScopeAuditWrite is the scope a service principal must hold to append audit events.
	ScopeAuditWrite = "audit.write"
)

// Principal is the caller identity forwarded by the gateway.
type Principal struct {
	ID     string
	Type   string
	Scopes []string
}

// Anonymous reports whether this is the gateway's anonymous principal.
func (p Principal) Anonymous() bool {
	return p.ID == "" || p.ID == anonymousID || p.Type == typeAnonymous
}

// IsService reports whether the principal is a service (not a user) principal.
func (p Principal) IsService() bool { return !p.Anonymous() && p.Type == typeService }

// HasScope reports whether the principal holds scope s.
func (p Principal) HasScope(s string) bool {
	for _, have := range p.Scopes {
		if have == s {
			return true
		}
	}
	return false
}

type principalKey struct{}

// ContextWithPrincipal injects a principal into ctx (exported for tests).
func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext returns the principal injected by Unary, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Unary injects the forwarded principal (if any) into the request context.
func Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if id := first(md, mdPrincipalID); id != "" {
				ctx = ContextWithPrincipal(ctx, Principal{
					ID:     id,
					Type:   first(md, mdPrincipalType),
					Scopes: splitScopes(first(md, mdPrincipalScopes)),
				})
			}
		}
		return handler(ctx, req)
	}
}

// RequirePrincipal returns the authenticated (non-anonymous) principal or an
// Unauthenticated error.
func RequirePrincipal(ctx context.Context) (Principal, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok || p.Anonymous() {
		return Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

// RequireAdmin authenticates the caller and requires the admin scope
// (PermissionDenied otherwise).
func RequireAdmin(ctx context.Context) (Principal, error) {
	p, err := RequirePrincipal(ctx)
	if err != nil {
		return Principal{}, err
	}
	if !p.HasScope(ScopeAdmin) {
		return Principal{}, status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", ScopeAdmin)
	}
	return p, nil
}

// RequireService authenticates the caller and requires a principal of type
// service holding scope. Users (admins included), anonymous callers and
// scopeless services are refused: Unauthenticated when there is no principal,
// PermissionDenied otherwise.
func RequireService(ctx context.Context, scope string) (Principal, error) {
	p, err := RequirePrincipal(ctx)
	if err != nil {
		return Principal{}, err
	}
	if !p.IsService() {
		return Principal{}, status.Error(codes.PermissionDenied, "service principal required")
	}
	if !p.HasScope(scope) {
		return Principal{}, status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", scope)
	}
	return p, nil
}

func first(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func splitScopes(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
