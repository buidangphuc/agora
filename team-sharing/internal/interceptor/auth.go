// Package interceptor holds the gRPC auth/identity plumbing for team-sharing.
//
// The gateway verifies the caller's JWT once and forwards the resulting
// principal as x-principal-{id,type,scopes} metadata (ADR-0003); this package
// reads exactly that, like every other service. Share links are public
// artifacts: ResolveShareLink is intentionally anonymous (anyone with the short
// code can unfurl it), and CreateShareLink accepts anonymous callers but stamps
// the real principal as the creator when one is present.
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

// Unary injects the forwarded principal (if any) into the request context so
// handlers resolve identity from context rather than re-reading metadata.
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

// CallerID returns the authenticated caller id, or "" for anonymous callers.
func CallerID(ctx context.Context) string {
	p, ok := PrincipalFromContext(ctx)
	if !ok || p.Anonymous() {
		return ""
	}
	return p.ID
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
