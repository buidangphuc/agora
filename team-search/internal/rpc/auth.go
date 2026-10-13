package rpc

import (
	"context"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
)

// requireScopes denies unless the forwarded principal holds every scope in want.
// No principal is Unauthenticated; a missing scope is PermissionDenied. The
// anonymous principal passes when it holds the scopes: the gateway gives
// anonymous callers the public scopes (search:read) so that browsing stays open.
func requireScopes(ctx context.Context, want ...string) error {
	_, err := auth.RequireScopesAllowAnonymous(ctx, want...)
	return err
}

// principalFromContext returns the forwarded principal in its wire form.
func principalFromContext(ctx context.Context) (*commonv1.Principal, bool) {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return nil, false
	}
	return &commonv1.Principal{Id: p.ID, Type: wirePrincipalType(p.Type), Scopes: p.Scopes}, true
}

func wirePrincipalType(t auth.PrincipalType) commonv1.PrincipalType {
	switch t {
	case auth.TypeUser:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case auth.TypeService:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case auth.TypeAnonymous:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	default:
		return commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	}
}
