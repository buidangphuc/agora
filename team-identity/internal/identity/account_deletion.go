package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"

	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

var (
	ErrDeletionPasswordMismatch = errors.New("password does not match")
	ErrObligationsUnavailable   = errors.New("obligations unavailable")
	ErrNoPendingDeletion        = store.ErrNoPendingDeletion
)

type DeletionBlockedError struct {
	Categories []string
}

func (e *DeletionBlockedError) Error() string {
	return "account deletion blocked by: " + strings.Join(e.Categories, ", ")
}

type DeletionRepository interface {
	GetByID(ctx context.Context, id string) (store.User, error)
	ScheduleDeletion(ctx context.Context, userID, keepSessionID string, now time.Time, window time.Duration) (store.DeletionSchedule, error)
	CancelDeletion(ctx context.Context, userID string) error
	DeletionEraseAfter(ctx context.Context, userID string) (time.Time, error)
}

type ObligationSource interface {
	UserObligations(ctx context.Context, userID string) (upstream.UserObligations, error)
	PayoutObligations(ctx context.Context, userID string) (upstream.PayoutObligations, error)
}

type AccountDeletionDeps struct {
	Users       DeletionRepository
	Obligations ObligationSource
	Window      time.Duration
	Now         func() time.Time
	Gate        *auditx.Gate
}

type AccountDeletionService struct {
	users       DeletionRepository
	obligations ObligationSource
	window      time.Duration
	now         func() time.Time
	gate        *auditx.Gate
}

func NewAccountDeletionService(deps AccountDeletionDeps) *AccountDeletionService {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	gate := deps.Gate
	if gate == nil {
		gate = auditx.NewGate(nil, nil)
	}
	return &AccountDeletionService{users: deps.Users, obligations: deps.Obligations, window: deps.Window, now: now, gate: gate}
}

func (s *AccountDeletionService) Request(ctx context.Context, userID, sessionID, password string) (time.Time, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return time.Time{}, ErrDeletionPasswordMismatch
	}
	uo, err := s.obligations.UserObligations(ctx, userID)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: order: %v", ErrObligationsUnavailable, err)
	}
	po, err := s.obligations.PayoutObligations(ctx, userID)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: payment: %v", ErrObligationsUnavailable, err)
	}
	var cats []string
	if uo.OpenBuyerOrders > 0 || uo.OpenSellerOrders > 0 {
		cats = append(cats, "open orders")
	}
	if uo.OpenReturns > 0 {
		cats = append(cats, "open returns")
	}
	if po.AvailableBalance != 0 || po.HeldBalance != 0 {
		cats = append(cats, "wallet balance")
	}
	if po.PendingPayouts > 0 {
		cats = append(cats, "pending payouts")
	}
	if len(cats) > 0 {
		return time.Time{}, &DeletionBlockedError{Categories: cats}
	}
	sch, err := s.users.ScheduleDeletion(ctx, userID, sessionID, s.now(), s.window)
	if err != nil {
		return time.Time{}, err
	}
	return sch.EraseAfter, nil
}

func (s *AccountDeletionService) Cancel(ctx context.Context, userID string) error {
	return s.users.CancelDeletion(ctx, userID)
}

func (s *AccountDeletionService) Status(ctx context.Context, userID string) (time.Time, error) {
	return s.users.DeletionEraseAfter(ctx, userID)
}

func (s *AccountDeletionService) Audited(ctx context.Context, actorID, action string, fn func(context.Context) error) error {
	ev := auditx.Event{ActorID: actorID, Action: action, TargetType: "user", TargetID: actorID}
	return s.gate.Do(ctx, ev, fn)
}
