package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type SubscriptionPeriod struct {
	PlanID             string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	AutoRenew          bool
	PendingPlanID      string
	PeriodSeq          int64
}

type SubscriptionTx interface {
	Get(ctx context.Context) (SellerSubscription, error)
	UpdatePeriod(ctx context.Context, p SubscriptionPeriod) (SellerSubscription, error)
}

type pgSubscriptionTx struct {
	tx       pgx.Tx
	sellerID string
}

func (t *pgSubscriptionTx) Get(ctx context.Context) (SellerSubscription, error) {
	if _, err := t.tx.Exec(ctx,
		`INSERT INTO seller_subscriptions (id, seller_id, plan_id, status) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (seller_id) DO NOTHING`,
		NewID(), t.sellerID, FreePlan().ID, SubscriptionStatusExpired); err != nil {
		return SellerSubscription{}, fmt.Errorf("insert subscription placeholder: %w", err)
	}
	q := `SELECT ` + subscriptionCols + `
		FROM seller_subscriptions s
		JOIN subscription_plans p ON p.id = s.plan_id
		WHERE s.seller_id = $1 FOR UPDATE OF s`
	var out SellerSubscription
	if err := scanSubscription(t.tx.QueryRow(ctx, q, t.sellerID), &out); err != nil {
		return SellerSubscription{}, fmt.Errorf("lock subscription for seller %q: %w", t.sellerID, err)
	}
	return out, nil
}

func (t *pgSubscriptionTx) UpdatePeriod(ctx context.Context, p SubscriptionPeriod) (SellerSubscription, error) {
	if _, err := t.Get(ctx); err != nil {
		return SellerSubscription{}, err
	}
	var pending *string
	if p.PendingPlanID != "" {
		pending = &p.PendingPlanID
	}
	if _, err := t.tx.Exec(ctx,
		`UPDATE seller_subscriptions SET plan_id = $2, status = $3, current_period_start = $4,
		 current_period_end = $5, auto_renew = $6, pending_plan_id = $7, period_seq = $8
		 WHERE seller_id = $1`,
		t.sellerID, p.PlanID, p.Status, p.CurrentPeriodStart, p.CurrentPeriodEnd, p.AutoRenew, pending, p.PeriodSeq); err != nil {
		return SellerSubscription{}, fmt.Errorf("update subscription period: %w", err)
	}
	return t.Get(ctx)
}

func (r *PostgresSubscriptionRepository) WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx SubscriptionTx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, &pgSubscriptionTx{tx: tx, sellerID: sellerID}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription tx: %w", err)
	}
	return nil
}

func (r *PostgresSubscriptionRepository) ListDueForRenewal(ctx context.Context, now time.Time, limit int) ([]SellerSubscription, error) {
	q := `SELECT ` + subscriptionCols + `
		FROM seller_subscriptions s
		JOIN subscription_plans p ON p.id = s.plan_id
		WHERE s.status = $1 AND s.current_period_end <= $2
		ORDER BY s.current_period_end ASC, s.id ASC
		LIMIT $3
		FOR UPDATE OF s SKIP LOCKED`
	rows, err := r.pool.Query(ctx, q, SubscriptionStatusActive, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due for renewal: %w", err)
	}
	defer rows.Close()
	var out []SellerSubscription
	for rows.Next() {
		var s SellerSubscription
		if err := scanSubscription(rows, &s); err != nil {
			return nil, fmt.Errorf("scan due subscription: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
