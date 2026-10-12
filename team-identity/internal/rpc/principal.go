package rpc

import (
	"context"

	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
)

// Principal types the gateway forwards in x-principal-type.
const (
	principalTypeUser      = "user"
	principalTypeService   = "service"
	principalTypeAnonymous = "anonymous"
)

// Gateway-computed audit metadata. It is never part of the principal and never
// used for authorization.
const (
	mdClientIP        = "x-client-ip"
	mdClientUserAgent = "x-client-user-agent"
)

// wirePrincipal converts the forwarded principal to its wire form, for the event
// envelope.
func wirePrincipal(p auth.Principal) *commonv1.Principal {
	return &commonv1.Principal{Id: p.ID, Type: wirePrincipalType(p.Type), Scopes: p.Scopes}
}

func wirePrincipalType(t string) commonv1.PrincipalType {
	switch t {
	case principalTypeUser:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case principalTypeService:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case principalTypeAnonymous:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	}
}

// clientFromContext reads the gateway-computed x-client-ip / x-client-user-agent
// metadata for the session and login records. Missing values are empty.
func clientFromContext(ctx context.Context) identity.ClientInfo {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return identity.ClientInfo{}
	}
	return identity.ClientInfo{IP: firstMD(md, mdClientIP), UserAgent: firstMD(md, mdClientUserAgent)}
}

func firstMD(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}
