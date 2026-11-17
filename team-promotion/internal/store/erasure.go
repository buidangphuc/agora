package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

type PostgresErasureRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresErasureRepository(pool *pgxpool.Pool) *PostgresErasureRepository {
	return &PostgresErasureRepository{pool: pool}
}

func (r *PostgresErasureRepository) EraseUser(ctx context.Context, userID string, at time.Time) error {
	return postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE vouchers SET active = false, deactivated_at = COALESCE(deactivated_at, $2), updated_at = now()
			WHERE owner_user_id = $1 AND (active OR deactivated_at IS NULL)`, userID, at); err != nil {
			return fmt.Errorf("deactivate owned vouchers: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM new_user_voucher_state WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("delete voucher state: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE seller_subscriptions
			SET plan_id = $2, status = CASE WHEN status = $3 THEN $4 ELSE status END, auto_renew = false, pending_plan_id = NULL
			WHERE seller_id = $1 AND (status = $3 OR auto_renew OR pending_plan_id IS NOT NULL)`,
			userID, FreePlan().ID, SubscriptionStatusActive, SubscriptionStatusCancelled); err != nil {
			return fmt.Errorf("end subscription: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE ad_campaigns SET status = $2 WHERE seller_id = $1 AND status IN ($3, $4, $5)`,
			userID, AdCampaignStatusEnded, AdCampaignStatusActive, AdCampaignStatusPaused, AdCampaignStatusPausedBudget); err != nil {
			return fmt.Errorf("end ad campaigns: %w", err)
		}
		return nil
	})
}
