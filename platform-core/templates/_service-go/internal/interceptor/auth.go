// Package interceptor holds the cross-cutting gRPC server interceptors every
// endpoint in this service runs through: authn/authz (this file) and tracing
// (tracing.go).
package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	// Generated from the vendored proto module — available after `make proto`.
	// See proto-vendor/README.md. Until then this import won't resolve.
	commonv1 "github.com/your-org/team-service/generated/platform/common/v1"
)

// principalCtxKey is an unexported type so the context key can't collide with
// keys set by other packages. Retrieve the value via PrincipalFromContext.
type principalCtxKey struct{}

// mdAuthorization is the gRPC metadata key carrying the bearer token.
// gRPC lowercases all metadata keys, so read it lowercased.
const mdAuthorization = "authorization"

// AuthConfig configures the auth interceptors.
type AuthConfig struct {
	// BearerToken is the expected shared secret (from AUTH_BEARER_TOKEN).
	//
	// TODO(ADR-0003): this is a stub. The real auth model resolves a Principal
	// at the edge from a JWT or session cookie and forwards it — bearer here is
	// only a placeholder so the wiring is exercisable end-to-end. Swap this for
	// JWT verification (issuer/audience/exp) or a cookie->session lookup.
	BearerToken string
}

// NewAuthConfig builds AuthConfig from the process environment.
func NewAuthConfig(bearerToken string) AuthConfig {
	return AuthConfig{BearerToken: bearerToken}
}

// PrincipalFromContext returns the Principal placed on the context by the auth
// interceptor. The bool is false when no Principal is present (e.g. a call that
// bypassed the interceptor, or an unauthenticated path).
func PrincipalFromContext(ctx context.Context) (*commonv1.Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*commonv1.Principal)
	return p, ok
}

// contextWithPrincipal returns a child context carrying p.
func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// authenticate validates the incoming credentials and returns the resolved
// Principal. It is shared by the unary and stream interceptors.
func (c AuthConfig) authenticate(ctx context.Context) (*commonv1.Principal, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	token, err := bearerToken(md)
	if err != nil {
		return nil, err
	}

	// Stub validation: constant-secret compare. Replace per ADR-0003.
	if c.BearerToken == "" || token != c.BearerToken {
		return nil, status.Error(codes.Unauthenticated, "invalid bearer token")
	}

	// A real edge would populate id/scopes from the verified token claims.
	return &commonv1.Principal{
		Id:     "svc:" + "team-service",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE,
		Scopes: []string{"listing.read"},
	}, nil
}

// bearerToken extracts the token from an `authorization: bearer <token>` header.
func bearerToken(md metadata.MD) (string, error) {
	vals := md.Get(mdAuthorization)
	if len(vals) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization metadata")
	}
	const prefix = "bearer "
	// Case-insensitive scheme match ("Bearer", "bearer", ...).
	raw := vals[0]
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", status.Error(codes.Unauthenticated, "expected 'bearer <token>' authorization")
	}
	return strings.TrimSpace(raw[len(prefix):]), nil
}

// UnaryServerInterceptor authenticates the caller and attaches the Principal to
// the context for downstream handlers.
func (c AuthConfig) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		p, err := c.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		return handler(contextWithPrincipal(ctx, p), req)
	}
}

// StreamServerInterceptor is the streaming counterpart. It wraps the
// ServerStream so handlers see the Principal on stream.Context().
func (c AuthConfig) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		p, err := c.authenticate(ss.Context())
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: contextWithPrincipal(ss.Context(), p)})
	}
}

// wrappedStream overrides Context() so the augmented context propagates to the
// streaming handler.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// RequireScopes returns codes.PermissionDenied with an `insufficient_scope`
// message unless the context Principal holds every scope in want. Call it at
// the top of a handler that needs coarse-grained RBAC:
//
//	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
//	    return nil, err
//	}
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
			// `insufficient_scope` mirrors platform.common.v1.Error.code convention.
			return status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", s)
		}
	}
	return nil
}
