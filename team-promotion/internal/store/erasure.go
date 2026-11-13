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
		return nil
	})
}
