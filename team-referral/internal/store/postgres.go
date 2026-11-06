package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

type PostgresReferralRepo struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPostgresReferralRepo(pool *pgxpool.Pool) *PostgresReferralRepo {
	ob, err := outbox.NewStore(pool, OutboxTable)
	if err != nil {
		panic(fmt.Sprintf("store: outbox over %s: %v", OutboxTable, err))
	}
	return &PostgresReferralRepo{pool: pool, outbox: ob}
}

func (r *PostgresReferralRepo) inTx(ctx context.Context, fn func(tx pgx.Tx) error) (err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresReferralRepo) GetCodeByUser(ctx context.Context, userID string) (string, error) {
	var code string
	err := r.pool.QueryRow(ctx, `SELECT code FROM referral_codes WHERE user_id = $1`, userID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

func (r *PostgresReferralRepo) CreateCode(ctx context.Context, userID, code string) (string, error) {
	const q = `
		INSERT INTO referral_codes (user_id, code)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING code`
	var stored string
	if err := r.pool.QueryRow(ctx, q, userID, code).Scan(&stored); err != nil {
		return "", err
	}
	return stored, nil
}

func (r *PostgresReferralRepo) OwnerOfCode(ctx context.Context, code string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM referral_codes WHERE code = $1`, code).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

func (r *PostgresReferralRepo) GetReferral(ctx context.Context, refereeID string) (Referral, error) {
	var ref Referral
	err := r.pool.QueryRow(ctx, `
		SELECT referrer_id, referee_id, code, status, attributed_at
		FROM referrals WHERE referee_id = $1`, refereeID).
		Scan(&ref.ReferrerID, &ref.RefereeID, &ref.Code, &ref.Status, &ref.AttributedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Referral{}, ErrReferralNotFound
	}
	return ref, err
}

func (r *PostgresReferralRepo) RegisteredAt(ctx context.Context, userID string) (time.Time, error) {
	var at time.Time
	err := r.pool.QueryRow(ctx, `SELECT registered_at FROM referral_accounts WHERE user_id = $1`, userID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotRegistered
	}
	return at, err
}

func (r *PostgresReferralRepo) HasCompletedOrder(ctx context.Context, buyerID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referral_buyer_orders WHERE buyer_id = $1)`, buyerID).Scan(&exists)
	return exists, err
}

func (r *PostgresReferralRepo) CountReferralsByReferrer(ctx context.Context, referrerID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM referrals WHERE referrer_id = $1`, referrerID).Scan(&n)
	return n, err
}

func (r *PostgresReferralRepo) RewardTotals(ctx context.Context, userID string) (RewardTotals, error) {
	var t RewardTotals
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount) FILTER (WHERE status = 'PENDING'), 0),
		       COALESCE(SUM(amount) FILTER (WHERE status = 'RELEASED'), 0)
		FROM referral_rewards WHERE user_id = $1`, userID).Scan(&t.PendingXU, &t.ReleasedXU)
	return t, err
}

func (r *PostgresReferralRepo) ListRewards(ctx context.Context, userID string, limit, offset int) ([]*referralv1.ReferralReward, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, amount, reason, created_at, status, release_at
		FROM referral_rewards
		WHERE user_id = $1 AND status IN ('PENDING', 'RELEASED')
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*referralv1.ReferralReward
	for rows.Next() {
		var rw referralv1.ReferralReward
		var createdAt, releaseAt time.Time
		var status string
		if err := rows.Scan(&rw.Id, &rw.Amount, &rw.Reason, &createdAt, &status, &releaseAt); err != nil {
			return nil, err
		}
		rw.CreatedAt = timestamppb.New(createdAt)
		rw.ReleaseAt = timestamppb.New(releaseAt)
		rw.Status = RewardStatusProto(status)
		out = append(out, &rw)
	}
	return out, rows.Err()
}

func (r *PostgresReferralRepo) GetRewardByOrder(ctx context.Context, orderID string) (Reward, error) {
	var rw Reward
	var referee, order *string
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, referee_id, order_id, reason, status, amount, created_at, release_at, released_at
		FROM referral_rewards WHERE order_id = $1`, orderID).
		Scan(&rw.ID, &rw.ReferrerID, &referee, &order, &rw.Reason, &rw.Status, &rw.AmountXU, &rw.CreatedAt, &rw.ReleaseAt, &rw.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reward{}, ErrRewardNotFound
	}
	if referee != nil {
		rw.RefereeID = *referee
	}
	if order != nil {
		rw.OrderID = *order
	}
	return rw, err
}

func (r *PostgresReferralRepo) PurgeProcessedEvents(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM referral_processed_events WHERE processed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
