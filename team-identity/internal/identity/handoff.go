package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handoffstatus"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	defaultHandoffTTL = 60 * time.Second
	maxLiveHandoffs   = 10
)

type HandoffRepository interface {
	InsertHandoffCode(ctx context.Context, c store.HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error
	RedeemHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time, inTx func(context.Context, store.HandoffCode) error) (store.HandoffCode, error)
	PeekHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time) (store.HandoffCode, error)
}

type PeekResult struct {
	Username  string
	ExpiresAt time.Time
}

const handoffCodeLen = 43

var errHandoffRejected = errors.New("handoff rejected")

func errHandoffUnauthenticated() error {
	return handoffstatus.InvalidCode()
}

type MintResult struct {
	Code      string
	ExpiresAt time.Time
}

func (s *AuthService) WithHandoff(r HandoffRepository) *AuthService {
	s.handoff = r
	return s
}

func (s *AuthService) WithHandoffTTL(d time.Duration) *AuthService {
	s.handoffTTL = d
	return s
}

func (s *AuthService) Mint(ctx context.Context, sourceSID string, target identityv1.HandoffApp) (MintResult, error) {
	p, ok := auth.FromContext(ctx)
	if !ok || p.Type == auth.TypeAnonymous || p.ID == "" {
		return MintResult{}, handoffstatus.AuthRequired()
	}
	if p.Type != auth.TypeUser {
		return MintResult{}, handoffstatus.UsersOnly()
	}
	if target == identityv1.HandoffApp_HANDOFF_APP_UNSPECIFIED {
		return MintResult{}, ErrInvalidInput
	}
	if _, known := identityv1.HandoffApp_name[int32(target)]; !known {
		return MintResult{}, ErrInvalidInput
	}
	if s.handoff == nil {
		return MintResult{}, handoffstatus.NotConfigured()
	}
	u, err := s.repo.GetByID(ctx, p.ID)
	if err != nil {
		return MintResult{}, err
	}
	u, err = s.liftIfExpired(ctx, u)
	if err != nil {
		return MintResult{}, err
	}
	if u.Status != store.StatusActive {
		return MintResult{}, ErrAccountLocked
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return MintResult{}, err
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(code))
	now := s.clock()
	ttl := defaultHandoffTTL
	if s.handoffTTL > 0 {
		ttl = s.handoffTTL
	}
	expires := now.Add(ttl)
	row := store.HandoffCode{
		CodeHash:  sum[:],
		UserID:    u.ID,
		SourceSID: sourceSID,
		TargetApp: target.String(),
		ExpiresAt: expires,
		CreatedAt: now,
	}
	ev := auditx.Event{
		ActorID:    u.ID,
		Action:     "auth.handoff_mint",
		TargetType: "user",
		TargetID:   u.ID,
		Metadata:   map[string]string{"target_app": target.String()},
	}
	err = s.handoff.InsertHandoffCode(ctx, row, maxLiveHandoffs, now, func(txCtx context.Context) error {
		return s.gate.Do(txCtx, ev, func(context.Context) error { return nil })
	})
	if err != nil {
		return MintResult{}, err
	}
	return MintResult{Code: code, ExpiresAt: expires}, nil
}

func (s *AuthService) handoffPrecheck(code string, target identityv1.HandoffApp) ([]byte, error) {
	if len(code) != handoffCodeLen || s.handoff == nil {
		return nil, errHandoffUnauthenticated()
	}
	if _, known := identityv1.HandoffApp_name[int32(target)]; !known || target == identityv1.HandoffApp_HANDOFF_APP_UNSPECIFIED {
		return nil, errHandoffUnauthenticated()
	}
	sum := sha256.Sum256([]byte(code))
	return sum[:], nil
}

func (s *AuthService) handoffEligible(ctx context.Context, c store.HandoffCode, lift bool) (store.User, error) {
	u, err := s.repo.GetByID(ctx, c.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.User{}, errHandoffRejected
		}
		return store.User{}, err
	}
	if lift {
		if u, err = s.liftIfExpired(ctx, u); err != nil {
			return store.User{}, err
		}
	}
	if u.Status != store.StatusActive {
		return store.User{}, errHandoffRejected
	}
	sessions, err := s.sessions.ListSessions(ctx, c.UserID)
	if err != nil {
		return store.User{}, err
	}
	for _, sess := range sessions {
		if sess.ID == c.SourceSID && !sess.Revoked {
			return u, nil
		}
	}
	return store.User{}, errHandoffRejected
}

func (s *AuthService) Redeem(ctx context.Context, code string, target identityv1.HandoffApp) (AuthResult, error) {
	hash, err := s.handoffPrecheck(code, target)
	if err != nil {
		return AuthResult{}, err
	}
	var res AuthResult
	_, err = s.handoff.RedeemHandoffCode(ctx, hash, target.String(), s.clock(), func(txCtx context.Context, c store.HandoffCode) error {
		u, err := s.handoffEligible(txCtx, c, true)
		if err != nil {
			return err
		}
		ev := auditx.Event{
			ActorID:    c.UserID,
			Action:     "auth.handoff_redeem",
			TargetType: "user",
			TargetID:   c.UserID,
			Metadata:   map[string]string{"target_app": target.String(), "result": "ok"},
		}
		return s.gate.Do(txCtx, ev, func(ctx context.Context) error {
			r, err := s.issue(ctx, u)
			if err != nil {
				return err
			}
			res = r
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, store.ErrHandoffInvalid) || errors.Is(err, errHandoffRejected) {
			return AuthResult{}, errHandoffUnauthenticated()
		}
		return AuthResult{}, err
	}
	return res, nil
}

func (s *AuthService) Peek(ctx context.Context, code string, target identityv1.HandoffApp) (PeekResult, error) {
	hash, err := s.handoffPrecheck(code, target)
	if err != nil {
		return PeekResult{}, err
	}
	c, err := s.handoff.PeekHandoffCode(ctx, hash, target.String(), s.clock())
	if err == nil {
		var u store.User
		if u, err = s.handoffEligible(ctx, c, false); err == nil {
			return PeekResult{Username: u.Username, ExpiresAt: c.ExpiresAt}, nil
		}
	}
	if errors.Is(err, store.ErrHandoffInvalid) || errors.Is(err, errHandoffRejected) {
		return PeekResult{}, errHandoffUnauthenticated()
	}
	return PeekResult{}, err
}
