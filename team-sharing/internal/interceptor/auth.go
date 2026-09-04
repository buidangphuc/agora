// Package interceptor holds the gRPC auth/identity plumbing for team-sharing.
//
// Share links are public artifacts: ResolveShareLink is intentionally anonymous
// (anyone with the short code can unfurl it), and CreateShareLink stamps the
// caller when one is present but still accepts anonymous callers. In production
// the principal is derived from a verified JWT at the gateway (ADR-0003); until
// that interceptor is wired end-to-end this reads a forwarded principal header
// as a placeholder so the layering and call sites are already in place.
package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type principalKey struct{}

// principalHeader is the forwarded, gateway-verified caller id. It is NOT trusted
// as an auth boundary here — the gateway verifies the JWT and forwards identity.
const principalHeader = "x-user-id"

// Unary injects the caller principal (if any) into the request context so
// handlers resolve identity from context rather than re-reading metadata.
func Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(principalHeader); len(vals) > 0 && vals[0] != "" {
				ctx = context.WithValue(ctx, principalKey{}, vals[0])
			}
		}
		return handler(ctx, req)
	}
}

// CallerID returns the authenticated caller id, or "" for anonymous callers.
func CallerID(ctx context.Context) string {
	if v, ok := ctx.Value(principalKey{}).(string); ok {
		return v
	}
	return ""
}
