// Package interceptor holds the cross-cutting gRPC server interceptors every
// endpoint runs through: principal resolution (this file) and tracing
// (tracing.go). Per ADR-0003, the Gateway verifies the token at the edge and
// forwards a RESOLVED Principal as trusted metadata; this service just reads it
// (no credentials, no JWT). It also owns RequireScopes, the RBAC helper handlers
// call.
package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
)

// Gateway-forwarded Principal metadata keys (ADR-0003). gRPC lowercases keys.
const (
	mdPrincipalID     = "x-principal-id"
	mdPrincipalType   = "x-principal-type"
	mdPrincipalScopes = "x-principal-scopes"
)

type principalCtxKey struct{}

// PrincipalFromContext returns the Principal the interceptor resolved.
func PrincipalFromContext(ctx context.Context) (*commonv1.Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*commonv1.Principal)
	return p, ok
}

func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// principalFromMetadata builds a Principal from the gateway-forwarded metadata.
// Returns nil when no principal was forwarded (e.g. a call that bypassed the
// gateway) — RequireScopes then denies as Unauthenticated.
func principalFromMetadata(ctx context.Context) *commonv1.Principal {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	id := firstMD(md, mdPrincipalID)
	if id == "" {
		return nil
	}
	return &commonv1.Principal{
		Id:     id,
		Type:   parsePrincipalType(firstMD(md, mdPrincipalType)),
		Scopes: splitScopes(firstMD(md, mdPrincipalScopes)),
	}
}

func firstMD(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func splitScopes(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePrincipalType(t string) commonv1.PrincipalType {
	switch t {
	case "user":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case "service":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case "anonymous":
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	}
}

// UnaryServerInterceptor attaches the forwarded Principal to the context.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if p := principalFromMetadata(ctx); p != nil {
			ctx = contextWithPrincipal(ctx, p)
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor is the streaming counterpart.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		if p := principalFromMetadata(ctx); p != nil {
			ctx = contextWithPrincipal(ctx, p)
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// wrappedStream overrides Context() so the augmented context reaches the handler.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// RequireScopes returns codes.PermissionDenied with an `insufficient_scope`
// message unless the context Principal holds every scope in want.
func RequireScopes(ctx context.Context, want ...string) error {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no principal on context")
	}
	have := make(map[string]struct{}, len(p.GetScopes()))
	for _, s := range p.GetScopes() {
		have[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := have[s]; !ok {
			return status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", s)
		}
	}
	return nil
}
