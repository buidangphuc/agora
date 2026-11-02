package identity

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

type PeekResult struct {
	Username  string
	ExpiresAt time.Time
}

func (s *AuthService) Redeem(context.Context, string, identityv1.HandoffApp) (AuthResult, error) {
	return AuthResult{}, status.Error(codes.Unimplemented, "TEST-STUB: task 1.2.2 not implemented")
}

func (s *AuthService) Peek(context.Context, string, identityv1.HandoffApp) (PeekResult, error) {
	return PeekResult{}, status.Error(codes.Unimplemented, "TEST-STUB: task 1.2.2 not implemented")
}
