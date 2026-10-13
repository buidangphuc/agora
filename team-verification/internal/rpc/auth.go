package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
)

// authenticated returns the caller when it is an end user or a service.
// No principal, an anonymous one, or a type that is neither user nor service is
// Unauthenticated. The SDK's auth.RequirePrincipal alone also admits a missing
// or unknown type, which these RPCs have always denied.
func authenticated(ctx context.Context) (auth.Principal, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if p.Type != auth.TypeUser && p.Type != auth.TypeService {
		return auth.Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

// userPrincipal returns the caller only when it is an authenticated end user:
// Unauthenticated as authenticated says, and PermissionDenied for a service
// principal (user-owned RPCs are never called by services).
func userPrincipal(ctx context.Context) (auth.Principal, error) {
	p, err := authenticated(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if p.Type != auth.TypeUser {
		return auth.Principal{}, status.Error(codes.PermissionDenied, "user principal required")
	}
	return p, nil
}
