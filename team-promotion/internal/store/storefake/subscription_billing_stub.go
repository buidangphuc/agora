package storefake

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// STUB-T1: compile-only placeholders replaced by the implementer of T1 (tasks 1.2).

var errBillingNotImplemented = errors.New("subscription billing storefake: not implemented")

func (r *InMemorySubscriptionRepository) WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx store.SubscriptionTx) error) error {
	return errBillingNotImplemented
}

func (r *InMemorySubscriptionRepository) ListDueForRenewal(ctx context.Context, now time.Time, limit int) ([]store.SellerSubscription, error) {
	return nil, errBillingNotImplemented
}
