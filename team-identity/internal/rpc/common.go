package rpc

import (
	"context"

	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
)

func ClientFromContext(ctx context.Context) identity.ClientInfo {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return identity.ClientInfo{}
	}
	return identity.ClientInfo{IP: firstMD(md, "x-client-ip"), UserAgent: firstMD(md, "x-client-user-agent")}
}

func firstMD(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func withClient(ctx context.Context) context.Context {
	return identity.WithClient(ctx, ClientFromContext(ctx))
}

func commonPrincipal(p auth.Principal) *commonv1.Principal {
	t := commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	switch p.Type {
	case auth.TypeUser:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case auth.TypeService:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case auth.TypeAnonymous:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	}
	return &commonv1.Principal{Id: p.ID, Type: t, Scopes: p.Scopes}
}
