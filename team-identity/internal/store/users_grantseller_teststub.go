package store

import (
	"context"
	"errors"
	"time"
)

// TEST-STUB: compile-only placeholder for task 1.2; the implementer replaces this file.

var ErrNotActive = errors.New("account not active")

type SellerGrant struct {
	UserID    string
	Role      string
	GrantedAt time.Time
}

type SellerEnqueueFn func(SellerGrant) (OutboxRow, error)

type SellerGrantResult struct {
	User    User
	Granted bool
}

func (r *PostgresUserRepository) GrantSeller(ctx context.Context, userID, version string, now time.Time, enqueue SellerEnqueueFn) (SellerGrantResult, error) {
	return SellerGrantResult{}, errors.New("not implemented")
}
