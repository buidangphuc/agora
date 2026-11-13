package store

import (
	"context"
	"errors"
	"time"
)

// STUB-T1: compile-only placeholders replaced by the implementer of T1 (tasks 1.2).

var errBillingNotImplemented = errors.New("subscription billing store: not implemented")

// SubscriptionPeriod is the billing state written by UpdatePeriod.
type SubscriptionPeriod struct {
	PlanID             string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	AutoRenew          bool
	PendingPlanID      string
	PeriodSeq          int64
}

// SubscriptionTx is a seller's subscription row held under FOR UPDATE.
type SubscriptionTx interface {
	// Get returns the locked row; inserts a placeholder (PeriodSeq 0, not ACTIVE) when the seller has none.
	Get(ctx context.Context) (SellerSubscription, error)
	// UpdatePeriod writes plan/status/period/auto_renew/pending/period_seq on the locked row.
	UpdatePeriod(ctx context.Context, p SubscriptionPeriod) (SellerSubscription, error)
}

func (r *PostgresSubscriptionRepository) WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx SubscriptionTx) error) error {
	return errBillingNotImplemented
}

// ListDueForRenewal returns ACTIVE rows with current_period_end <= now, oldest first, at most limit,
// selected FOR UPDATE SKIP LOCKED (rows locked by another transaction are skipped).
func (r *PostgresSubscriptionRepository) ListDueForRenewal(ctx context.Context, now time.Time, limit int) ([]SellerSubscription, error) {
	return nil, errBillingNotImplemented
}
