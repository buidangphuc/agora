package rpc_test

import (
	"context"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
)

// contextWithPrincipal puts p in ctx the way the SDK auth interceptor does after
// reading the gateway's x-principal-* metadata.
func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	var typ auth.PrincipalType
	switch p.GetType() {
	case commonv1.PrincipalType_PRINCIPAL_TYPE_USER:
		typ = auth.TypeUser
	case commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE:
		typ = auth.TypeService
	case commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS:
		typ = auth.TypeAnonymous
	}
	return auth.WithPrincipal(ctx, auth.Principal{ID: p.GetId(), Type: typ, Scopes: p.GetScopes()})
}
