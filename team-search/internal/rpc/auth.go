package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
)

const (
	principalTypeUser      = "user"
	principalTypeService   = "service"
	principalTypeAnonymous = "anonymous"
)

// requireScopes denies unless the forwarded principal holds every scope in want.
// No principal is Unauthenticated; a missing scope is PermissionDenied.
//
// It does not use auth.RequireScopes on purpose: that helper treats the
// anonymous principal as unauthenticated, but the gateway gives anonymous
// callers the public scopes (search:read) so that browsing stays open.
func requireScopes(ctx context.Context, want ...string) error {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}
	for _, s := range want {
		if !p.HasScope(s) {
			return status.Error(codes.PermissionDenied, "missing scope")
		}
	}
	return nil
}

// principalFromContext returns the forwarded principal in its wire form.
func principalFromContext(ctx context.Context) (*commonv1.Principal, bool) {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return nil, false
	}
	return &commonv1.Principal{Id: p.ID, Type: wirePrincipalType(p.Type), Scopes: p.Scopes}, true
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
