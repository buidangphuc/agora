package identity

import (
	"context"
	"errors"
)

func (s *AuthService) WithSellerTerms(versions []string) *AuthService { return s }

func (s *AuthService) BecomeSeller(ctx context.Context, sid, termsVersion string) (AuthResult, error) {
	return AuthResult{}, errors.New("BecomeSeller not implemented")
}
