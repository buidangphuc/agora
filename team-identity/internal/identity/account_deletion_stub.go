package identity

// STUB (test-author, task t2-obligations-lifecycle-rpcs): the implementer replaces this file
// with the real account deletion service.

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

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
}

type AccountDeletionService struct{}

func NewAccountDeletionService(deps AccountDeletionDeps) *AccountDeletionService {
	return &AccountDeletionService{}
}

var errDeletionNotImplemented = errors.New("not implemented")

func (s *AccountDeletionService) Request(ctx context.Context, userID, sessionID, password string) (time.Time, error) {
	return time.Time{}, errDeletionNotImplemented
}

func (s *AccountDeletionService) Cancel(ctx context.Context, userID string) error {
	return errDeletionNotImplemented
}

func (s *AccountDeletionService) Status(ctx context.Context, userID string) (time.Time, error) {
	return time.Time{}, errDeletionNotImplemented
}
