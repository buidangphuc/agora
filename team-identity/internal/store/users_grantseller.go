package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotActive = errors.New("account not active")

const roleSeller = "seller"

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

func (r *PostgresUserRepository) GrantSeller(ctx context.Context, userID, version string, now time.Time, enqueue SellerEnqueueFn, inTx func(context.Context) error) (SellerGrantResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SellerGrantResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u User
	if err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 FOR UPDATE`, userID), &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellerGrantResult{}, ErrNotFound
		}
		return SellerGrantResult{}, fmt.Errorf("lock user: %w", err)
	}
	if u.Status != StatusActive {
		return SellerGrantResult{}, ErrNotActive
	}
	if slices.Contains(u.Roles, roleSeller) {
		return SellerGrantResult{User: u}, nil
	}

	const upd = `UPDATE users SET roles = array_append(roles, 'seller'), seller_terms_version = $2, seller_terms_accepted_at = $3
		WHERE id = $1 RETURNING ` + userColumns
	if err := scanUser(tx.QueryRow(ctx, upd, userID, version, now), &u); err != nil {
		return SellerGrantResult{}, fmt.Errorf("grant seller: %w", err)
	}
	row, err := enqueue(SellerGrant{UserID: userID, Role: roleSeller, GrantedAt: now})
	if err != nil {
		return SellerGrantResult{}, fmt.Errorf("build role event: %w", err)
	}
	if err := enqueueTx(ctx, r.ob, tx, row); err != nil {
		return SellerGrantResult{}, err
	}
	if inTx != nil {
		if err := inTx(ctx); err != nil {
			return SellerGrantResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SellerGrantResult{}, fmt.Errorf("commit tx: %w", err)
	}
	return SellerGrantResult{User: u, Granted: true}, nil
}
