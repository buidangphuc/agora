package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	defaultHandoffTTL = 60 * time.Second
	maxLiveHandoffs   = 10
)

type HandoffRepository interface {
	InsertHandoffCode(ctx context.Context, c store.HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error
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
		return MintResult{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	if p.Type != auth.TypeUser {
		return MintResult{}, status.Error(codes.PermissionDenied, "only users may mint a handoff code")
	}
	if target == identityv1.HandoffApp_HANDOFF_APP_UNSPECIFIED {
		return MintResult{}, ErrInvalidInput
	}
	if _, known := identityv1.HandoffApp_name[int32(target)]; !known {
		return MintResult{}, ErrInvalidInput
	}
	if s.handoff == nil {
		return MintResult{}, status.Error(codes.Unavailable, "handoff is not configured")
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
