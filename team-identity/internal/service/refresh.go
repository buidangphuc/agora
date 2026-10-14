package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/repository"
)

var (
	// ErrInvalidRefreshToken is the single generic refresh failure; it never reveals why.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrRefreshUnavailable is a transient database failure; the session is not revoked.
	ErrRefreshUnavailable = errors.New("refresh unavailable")

	errRefreshInternal = errors.New("refresh failed")
)

const (
	refreshGrace  = 10 * time.Second
	refreshMaxAge = 90 * 24 * time.Hour
)

// newRefreshToken mints 32 random bytes, base64url without padding (43 chars).
func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// revokedEvent builds the SessionRevoked outbox row for a session revoked by refresh.
func (s *AuthService) revokedEvent(ctx context.Context, now time.Time) repository.EnqueueFn {
	requestID := requestIDFromCtx(ctx)
	return func(revoked repository.Session) (repository.OutboxRow, error) {
		eventID := uuid.NewString()
		principal := &commonv1.Principal{Id: revoked.UserID, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
		payload, err := events.BuildSessionRevokedEnvelope(eventID, revoked.ID, revoked.UserID,
			now.Add(s.ttl), principal, requestID)
		if err != nil {
			return repository.OutboxRow{}, err
		}
		return repository.OutboxRow{
			EventID:       eventID,
			AggregateType: "Session",
			AggregateID:   revoked.UserID,
			EventType:     events.SessionRevokedEventType,
			Payload:       payload,
			RequestID:     requestID,
		}, nil
	}
}

// RefreshSession rotates a refresh token and issues a new access token bound to the
// same session. Tokens and hashes never appear in errors.
func (s *AuthService) RefreshSession(ctx context.Context, refreshToken string) (AuthResult, error) {
	if s.sessions == nil || refreshToken == "" {
		return AuthResult{}, ErrInvalidRefreshToken
	}
	newRaw, err := newRefreshToken()
	if err != nil {
		return AuthResult{}, errRefreshInternal
	}
	now := s.clock()
	ev := s.revokedEvent(ctx, now)
	res, err := s.sessions.RotateRefresh(ctx, hashToken(refreshToken), hashToken(newRaw), now, refreshGrace, refreshMaxAge, ev)
	if err != nil {
		return AuthResult{}, mapRotateErr(err)
	}
	switch res.Outcome {
	case repository.RotateRotated, repository.RotateReplayedInGrace:
	default:
		return AuthResult{}, ErrInvalidRefreshToken
	}

	u, err := s.repo.GetByID(ctx, res.Session.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			if rerr := s.sessions.RevokeSession(ctx, res.Session.ID, res.Session.UserID, ev); rerr != nil &&
				!errors.Is(rerr, repository.ErrSessionNotFound) {
				return AuthResult{}, errRefreshInternal
			}
			return AuthResult{}, ErrInvalidRefreshToken
		}
		return AuthResult{}, fmt.Errorf("%w: user lookup", ErrRefreshUnavailable)
	}
	scopes := authz.ScopesForRoles(u.Roles)
	signed, err := s.signer.SignWithSession(u.ID, u.Username, "user", scopes, res.Session.ID, s.ttl)
	if err != nil {
		return AuthResult{}, errRefreshInternal
	}
	out := AuthResult{Token: signed, UserID: u.ID, Username: u.Username, Type: "user", Scopes: scopes}
	if res.Outcome == repository.RotateRotated {
		out.RefreshToken, out.RefreshExpiresAt = newRaw, res.Session.RefreshExpiresAt
	}
	return out, nil
}

// mapRotateErr turns repository errors into non-leaking service errors. Database
// connectivity-style errors cannot be told apart from other faults here, so every
// RotateRefresh error is a fault (never ErrInvalidRefreshToken, never a revoke).
func mapRotateErr(err error) error {
	_ = err // deliberately dropped: may embed hash text
	return ErrRefreshUnavailable
}

// requestIDFromCtx reads the gateway-forwarded x-request-id (trace continuity only).
func requestIDFromCtx(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-request-id"); len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
