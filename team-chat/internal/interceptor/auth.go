package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
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
	// Reject the gateway's anonymous principal (id "anonymous", type ANONYMOUS):
	// an id != "" check alone let unauthenticated callers through.
	if !ok || p == nil || p.GetId() == "" ||
		p.GetId() == "anonymous" ||
		p.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}
