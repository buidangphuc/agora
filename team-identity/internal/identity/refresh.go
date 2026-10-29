package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	"github.com/buidangphuc/team-identity/internal/authz"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/store"
)

var (
	// ErrInvalidRefreshToken is the single generic refresh failure; it never reveals why.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrRefreshUnavailable is a transient database failure; the session is not revoked.
	ErrRefreshUnavailable = errors.New("refresh unavailable")

	// ErrRefreshInternal is a non-transient refresh fault (event build/outbox/conflict, signing).
	ErrRefreshInternal = errors.New("refresh failed")
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
func (s *AuthService) revokedEvent(ctx context.Context, now time.Time) store.EnqueueFn {
	requestID := requestIDFromCtx(ctx)
	return func(revoked store.Session) (store.OutboxRow, error) {
		eventID := uuid.NewString()
		principal := &commonv1.Principal{Id: revoked.UserID, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}
		payload, err := events.BuildSessionRevokedEnvelope(eventID, revoked.ID, revoked.UserID,
			now.Add(s.ttl), principal, requestID)
		if err != nil {
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
			EventID:       eventID,
			AggregateType: "User",
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
	if refreshToken == "" {
		return AuthResult{}, ErrInvalidRefreshToken
	}
	newRaw, err := newRefreshToken()
	if err != nil {
		return AuthResult{}, ErrRefreshInternal
	}
	now := s.clock()
	ev := s.revokedEvent(ctx, now)
	c, _ := ctx.Value(clientCtxKey{}).(ClientInfo)
	res, err := s.sessions.RotateRefresh(ctx, hashToken(refreshToken), hashToken(newRaw), now, refreshGrace, refreshMaxAge, clip(c.IP, 64), clip(c.UserAgent, 256), ev)
	if err != nil {
		return AuthResult{}, mapRotateErr(err)
	}
	switch res.Outcome {
	case store.RotateRotated, store.RotateReplayedInGrace:
	default:
		return AuthResult{}, ErrInvalidRefreshToken
	}

	u, err := s.repo.GetByID(ctx, res.Session.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if err := s.revokeRefreshed(ctx, res.Session, ev); err != nil {
				return AuthResult{}, err
			}
			return AuthResult{}, ErrInvalidRefreshToken
		}
		return AuthResult{}, fmt.Errorf("%w: user lookup", ErrRefreshUnavailable)
	}
	if u.Status == store.StatusSuspended || u.Status == store.StatusBanned {
		if err := s.revokeRefreshed(ctx, res.Session, ev); err != nil {
			return AuthResult{}, err
		}
		return AuthResult{}, ErrAccountLocked
	}
	scopes := authz.ScopesForUser(u.Roles, u.Status)
	signed, err := s.signer.SignWithSession(u.ID, u.Username, "user", scopes, res.Session.ID, s.ttl)
	if err != nil {
		return AuthResult{}, ErrRefreshInternal
	}
	out := AuthResult{Token: signed, UserID: u.ID, Username: u.Username, Type: "user", Scopes: scopes}
	if res.Outcome == store.RotateRotated {
		out.RefreshToken, out.RefreshExpiresAt = newRaw, res.Session.RefreshExpiresAt
	}
	return out, nil
}

// revokeRefreshed revokes the session a refresh just resolved; an already-gone session is fine.
func (s *AuthService) revokeRefreshed(ctx context.Context, sess store.Session, ev store.EnqueueFn) error {
	if rerr := s.sessions.RevokeSession(ctx, sess.ID, sess.UserID, ev); rerr != nil &&
		!errors.Is(rerr, store.ErrSessionNotFound) {
		if errors.Is(rerr, store.ErrRotateFault) {
			return ErrRefreshInternal
		}
		return ErrRefreshUnavailable
	}
	return nil
}

// mapRotateErr turns repository errors into non-leaking service errors. Database
// ErrRotateFault maps to ErrRefreshInternal; any other error is transient
// (ErrRefreshUnavailable). The cause is not wrapped, as it may embed hash text.
func mapRotateErr(err error) error {
	if errors.Is(err, store.ErrRotateFault) {
		return ErrRefreshInternal
	}
	return ErrRefreshUnavailable
}

// requestIDFromCtx reads the gateway-forwarded request id (trace continuity only).
func requestIDFromCtx(ctx context.Context) string { return reqctx.RequestID(ctx) }
