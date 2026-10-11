package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
)

const (
	mdPrincipalID     = "x-principal-id"
	mdPrincipalType   = "x-principal-type"
	mdPrincipalScopes = "x-principal-scopes"
)

type principalCtxKey struct{}

func PrincipalFromContext(ctx context.Context) (*commonv1.Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*commonv1.Principal)
	return p, ok
}

func ContextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

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

func AuthUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if p := principalFromMetadata(ctx); p != nil {
			ctx = ContextWithPrincipal(ctx, p)
		}
		return handler(ctx, req)
	}
}

func RequirePrincipal(ctx context.Context) (*commonv1.Principal, error) {
	p, ok := PrincipalFromContext(ctx)
	// The gateway resolves unauthenticated callers to an anonymous principal
	// (id "anonymous", type ANONYMOUS). Reject those: an id != "" check alone
	// let anonymous through, so write RPCs (CreateVoucher, flash-sale, ad
	// campaigns, subscriptions) were mint-able without logging in.
	if !ok || p == nil || p.GetId() == "" ||
		p.GetId() == "anonymous" ||
		p.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

const (
	// ScopeAdmin marks platform administrators (granted by team-identity to the admin role).
	ScopeAdmin = "admin"
	// ScopeListingWrite is granted to seller and admin roles, never to buyers.
	ScopeListingWrite = "listing.write"
	// ScopePromoReserve is service-only (held by service-team-order, granted to no
	// user role): it gates the voucher redemption saga RPCs.
	ScopePromoReserve = "promotion.reserve"
)

// RequireScopes authenticates the caller (none or anonymous is UNAUTHENTICATED) and
// then requires every scope in want (PERMISSION_DENIED otherwise). It returns the
// principal so the handler can bind identity from it.
func RequireScopes(ctx context.Context, want ...string) (*commonv1.Principal, error) {
	p, err := RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range want {
		if !HasScope(p, s) {
			return nil, status.Errorf(codes.PermissionDenied, "insufficient_scope: missing %q", s)
		}
	}
	return p, nil
}

// IsService reports whether p is a service principal (type SERVICE).
func IsService(p *commonv1.Principal) bool {
	return p.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
}

// RequireService demands a principal of type SERVICE that also holds scope. The
// type is checked, not only the scope string: a user principal that carries the
// scope is PERMISSION_DENIED.
func RequireService(ctx context.Context, scope string) (*commonv1.Principal, error) {
	p, err := RequireScopes(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !IsService(p) {
		return nil, status.Error(codes.PermissionDenied, "service principal required")
	}
	return p, nil
}

// HasScope reports whether p carries the given scope.
func HasScope(p *commonv1.Principal, scope string) bool {
	for _, s := range p.GetScopes() {
		if s == scope {
			return true
		}
	}
	return false
}

// IsAdmin reports whether p carries the admin scope.
func IsAdmin(p *commonv1.Principal) bool { return HasScope(p, ScopeAdmin) }

// RequireSeller allows seller and admin principals (listing.write or admin scope).
func RequireSeller(p *commonv1.Principal) error {
	if IsAdmin(p) || HasScope(p, ScopeListingWrite) {
		return nil
	}
	return status.Error(codes.PermissionDenied, "seller or admin role required")
}

// RequireAdmin allows only admin principals.
func RequireAdmin(p *commonv1.Principal) error {
	if IsAdmin(p) {
		return nil
	}
	return status.Error(codes.PermissionDenied, "admin role required")
}
