package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
)

// requireScopes denies unless the forwarded principal holds every scope in want.
// No principal is Unauthenticated; a missing scope is PermissionDenied.
//
// It does not use auth.RequireScopes on purpose: that helper treats the
// anonymous principal as unauthenticated, but the gateway gives anonymous
// callers the public scopes (listing.read) so that browsing stays open.
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

// requireServiceScope is requireScopes for service-to-service RPCs: the principal
// must be of type service and hold every scope in want. A user (even a seller
// holding the same scope) or an anonymous caller is denied, so an end user cannot
// reach an internal RPC just because the gateway forwards their token.
func requireServiceScope(ctx context.Context, want ...string) error {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}
	if p.Type != auth.TypeService {
		return status.Error(codes.PermissionDenied, "service principal required")
	}
	return requireScopes(ctx, want...)
}

// principalFromContext returns the forwarded principal in its wire form, for the
// event envelope.
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

// requestIDFromContext returns the correlation id of this call, if present.
func requestIDFromContext(ctx context.Context) (string, bool) {
	id := reqctx.RequestID(ctx)
	return id, id != ""
}
