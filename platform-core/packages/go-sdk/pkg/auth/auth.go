package auth

import (
	"context"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// Metadata keys the gateway uses to forward the resolved principal.
const (
	MDPrincipalID     = "x-principal-id"
	MDPrincipalType   = "x-principal-type"
	MDPrincipalScopes = "x-principal-scopes"
)

const anonymous = "anonymous"

// Principal is the caller identity forwarded by the gateway.
type Principal struct {
	ID     string
	Type   string
	Scopes []string
}

// Anonymous reports whether p is the gateway's anonymous principal.
func (p Principal) Anonymous() bool {
	return p.ID == "" || p.ID == anonymous || p.Type == anonymous
}

// HasScope reports whether p holds s by exact match.
func (p Principal) HasScope(s string) bool {
	return slices.Contains(p.Scopes, s)
}

type principalKey struct{}

// FromContext returns the principal stored by the interceptors or WithPrincipal.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// WithPrincipal returns ctx carrying p and its id as the reqctx principal id.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	ctx = context.WithValue(ctx, principalKey{}, p)
	return reqctx.WithPrincipalID(ctx, p.ID)
}

// UnaryServerInterceptor stores the forwarded principal in the handler context.
// Without x-principal-id nothing is set and the call proceeds.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(withMetadataPrincipal(ctx), req)
	}
}

// StreamServerInterceptor is the streaming counterpart of UnaryServerInterceptor.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &principalStream{ServerStream: ss, ctx: withMetadataPrincipal(ss.Context())})
	}
}

// principalStream overrides Context so the handler sees the enriched context.
type principalStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *principalStream) Context() context.Context { return s.ctx }

// RequirePrincipal returns the caller, or Unauthenticated when there is none or
// it is anonymous.
func RequirePrincipal(ctx context.Context) (Principal, error) {
	p, ok := FromContext(ctx)
	if !ok || p.Anonymous() {
		return Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

// RequireScopes calls RequirePrincipal, then demands every scope by exact match.
// It returns PermissionDenied when any is missing.
func RequireScopes(ctx context.Context, scopes ...string) (Principal, error) {
	p, err := RequirePrincipal(ctx)
	if err != nil {
		return Principal{}, err
	}
	for _, s := range scopes {
		if !p.HasScope(s) {
			return Principal{}, status.Error(codes.PermissionDenied, "missing scope")
		}
	}
	return p, nil
}

// RequireScopesAllowAnonymous is RequireScopes for public methods: an anonymous
// principal passes when it holds every scope. It returns Unauthenticated only
// when there is no principal, and PermissionDenied when any scope is missing.
func RequireScopesAllowAnonymous(ctx context.Context, scopes ...string) (Principal, error) {
	p, ok := FromContext(ctx)
	if !ok {
		return Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	for _, s := range scopes {
		if !p.HasScope(s) {
			return Principal{}, status.Error(codes.PermissionDenied, "missing scope")
		}
	}
	return p, nil
}

func withMetadataPrincipal(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	id := first(md, MDPrincipalID)
	if id == "" {
		return ctx
	}
	return WithPrincipal(ctx, Principal{
		ID:     id,
		Type:   first(md, MDPrincipalType),
		Scopes: splitScopes(first(md, MDPrincipalScopes)),
	})
}

func first(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func splitScopes(raw string) []string {
	out := []string{}
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
