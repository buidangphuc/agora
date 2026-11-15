package store

import (
	"context"
	"errors"
	"time"
)

const StatusDeleted = "deleted"

var ErrNoPendingDeletion = errors.New("no pending account deletion")

var errNotImplemented = errors.New("account deletion store: not implemented")

type DeletionSchedule struct {
	RequestedAt     time.Time
	EraseAfter      time.Time
	Created         bool
	SessionsRevoked int
}

type Erasure struct {
	UserID   string
	Roles    []string
	ErasedAt time.Time
}

type ErasedEnqueueFn func(Erasure) (OutboxRow, error)

type ErasureResult struct {
	Erased  int
	Failed  int
	Skipped int
}

func (r *PostgresUserRepository) ScheduleDeletion(ctx context.Context, userID, keepSessionID string, now time.Time, window time.Duration) (DeletionSchedule, error) {
	return DeletionSchedule{}, errNotImplemented
}

func (r *PostgresUserRepository) CancelDeletion(ctx context.Context, userID string) error {
	return errNotImplemented
}

func (r *PostgresUserRepository) EraseDue(ctx context.Context, now time.Time, limit int, enqueue ErasedEnqueueFn) (ErasureResult, error) {
	return ErasureResult{}, errNotImplemented
}
