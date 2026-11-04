package identity

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/store"
)

const roleGrantedEventType = "platform.identity.v1.UserRoleGranted"

type sellerGranter interface {
	GrantSeller(ctx context.Context, userID, version string, now time.Time, enqueue store.SellerEnqueueFn) (store.SellerGrantResult, error)
}

func (s *AuthService) WithSellerTerms(versions []string) *AuthService {
	s.sellerTerms = append([]string(nil), versions...)
	return s
}

func (s *AuthService) roleGrantedEvent(ctx context.Context) store.SellerEnqueueFn {
	requestID := requestIDFromCtx(ctx)
	return func(g store.SellerGrant) (store.OutboxRow, error) {
		return store.OutboxRow{
			EventID:       uuid.NewString(),
			AggregateType: "User",
			AggregateID:   g.UserID,
			EventType:     roleGrantedEventType,
			RequestID:     requestID,
		}, nil
	}
}

func (s *AuthService) BecomeSeller(ctx context.Context, sid, termsVersion string) (AuthResult, error) {
	if len(s.sellerTerms) == 0 || termsVersion != s.sellerTerms[0] {
		return AuthResult{}, ErrInvalidInput
	}
	p, ok := auth.FromContext(ctx)
	if !ok || p.ID == "" || sid == "" {
		return AuthResult{}, ErrInvalidInput
	}
	granter, ok := s.repo.(sellerGranter)
	if !ok {
		return AuthResult{}, errors.New("user repository does not support seller grant")
	}
	list, err := s.sessions.ListSessions(ctx, p.ID)
	if err != nil {
		return AuthResult{}, err
	}
	live := false
	for _, sess := range list {
		if sess.ID == sid && !sess.Revoked {
			live = true
			break
		}
	}
	if !live {
		return AuthResult{}, store.ErrSessionNotFound
	}
	res, err := granter.GrantSeller(ctx, p.ID, termsVersion, s.clock(), s.roleGrantedEvent(ctx))
	if err != nil {
		return AuthResult{}, err
	}
	u := res.User
	scopes := authz.ScopesForUser(u.Roles, u.Status)
	signed, err := s.signer.SignWithSession(u.ID, u.Username, "user", scopes, sid, u.Status, s.ttl)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: signed, UserID: u.ID, Username: u.Username, Type: "user", Scopes: scopes}, nil
}
