// Package interceptor resolves the calling principal at the gRPC edge and makes
// it available to handlers via context. In production the identity is minted by
// the platform's auth layer (ADR-0003); here the unary interceptor reads a
// resolved user id from request metadata (the "x-user-id" key the gateway sets
// after verifying the caller). Handlers never trust request bodies for identity
// — they read the principal from context only.
package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataUserIDKey is the gRPC metadata key carrying the resolved caller id.
// The gateway forwards the resolved principal under "x-principal-id" (see
// team-gateway/internal/edge/forward.go), matching the other services.
const MetadataUserIDKey = "x-principal-id"

// MetadataPrincipalTypeKey carries the principal type the gateway resolved. The
// gateway forwards callers without a bearer token as id "anonymous" with type
// "anonymous" (team-gateway/internal/edge/forward.go), so a non-empty id alone
// does not mean the caller is authenticated.
const MetadataPrincipalTypeKey = "x-principal-type"

type principalCtxKey struct{}

// Principal is the resolved identity of the caller. Anonymous is true when no
// verified user id accompanied the request; such callers may not touch
// user-owned data.
type Principal struct {
	UserID    string
	Anonymous bool
}

// FromContext returns the principal resolved by UnaryServerInterceptor. When no
// interceptor ran (or no id was present) it returns an anonymous principal.
func FromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalCtxKey{}).(Principal); ok {
		return p
	}
	return Principal{Anonymous: true}
}

// WithPrincipal injects a principal into ctx. Exported so tests can exercise
// handlers without spinning up a full gRPC pipeline.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// UnaryServerInterceptor extracts the caller id from request metadata and stores
// the resulting principal in the handler context.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = interceptorWithPrincipal(ctx)
		return handler(ctx, req)
	}
}

func interceptorWithPrincipal(ctx context.Context) context.Context {
	p := Principal{Anonymous: true}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		id := firstValue(md, MetadataUserIDKey)
		ptype := firstValue(md, MetadataPrincipalTypeKey)
		if id != "" && id != "anonymous" && ptype != "anonymous" {
			p = Principal{UserID: id, Anonymous: false}
		}
	}
	return WithPrincipal(ctx, p)
}

func firstValue(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}
