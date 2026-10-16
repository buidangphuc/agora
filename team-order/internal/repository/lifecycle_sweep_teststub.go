package repository

import (
	"context"
	"time"
)

// TEST-STUB (task 9, compile-only): the implementer replaces this file with the
// real code in order.go / return.go.

// WithReturnAutoApproveAfter stamps auto_approve_at = created + d on every return created.
func WithReturnAutoApproveAfter(d time.Duration) ReturnRepoOption {
	return func(*returnRepoConfig) {}
}

// SetReturnSource lets the in-memory order store evaluate the blocking-return
// rule of ListAutoCompletable (the Postgres store does it in SQL).
func (r *InMemoryOrderRepository) SetReturnSource(ReturnRepository) {}

// ListAutoCompletable: SHIPPED orders with auto_complete_at <= now and no blocking
// return (PENDING, or APPROVED when the order has paid_at), oldest deadline first.
func (r *InMemoryOrderRepository) ListAutoCompletable(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	return nil, nil
}

// ListAutoApprovable: PENDING returns with auto_approve_at <= now, oldest first.
func (r *InMemoryReturnRepository) ListAutoApprovable(ctx context.Context, now time.Time, limit int) ([]OrderReturn, error) {
	return nil, nil
}
