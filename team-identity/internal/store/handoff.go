package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrHandoffCapReached = errors.New("handoff cap reached")
	ErrHandoffInvalid    = errors.New("handoff code invalid")
)

const handoffPurgeRetention = 24 * time.Hour

type HandoffCode struct {
	CodeHash   []byte
	UserID     string
	SourceSID  string
	TargetApp  string
	ExpiresAt  time.Time
	RedeemedAt time.Time
	CreatedAt  time.Time
}

type PostgresHandoffRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresHandoffRepository(pool *pgxpool.Pool) *PostgresHandoffRepository {
	return &PostgresHandoffRepository{pool: pool}
}

func (r *PostgresHandoffRepository) InsertHandoffCode(ctx context.Context, c HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('handoff:' || $1::text, 0))`, c.UserID); err != nil {
		return err
	}
	var live int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM handoff_codes
		WHERE user_id = $1 AND redeemed_at IS NULL AND expires_at >= $2`, c.UserID, now).Scan(&live); err != nil {
		return err
	}
	if live >= maxLive {
		return ErrHandoffCapReached
	}
	if _, err := tx.Exec(ctx, `INSERT INTO handoff_codes (code_hash, user_id, source_sid, target_app, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, c.CodeHash, c.UserID, c.SourceSID, c.TargetApp, c.ExpiresAt, now); err != nil {
		return err
	}
	if inTx != nil {
		if err := inTx(ctx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresHandoffRepository) RedeemHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time, inTx func(context.Context, HandoffCode) error) (HandoffCode, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return HandoffCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c HandoffCode
	err = tx.QueryRow(ctx, `UPDATE handoff_codes SET redeemed_at = $3
		WHERE code_hash = $1 AND target_app = $2 AND redeemed_at IS NULL AND expires_at >= $3
		RETURNING code_hash, user_id, source_sid, target_app, expires_at, created_at`,
		codeHash, targetApp, now).Scan(&c.CodeHash, &c.UserID, &c.SourceSID, &c.TargetApp, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffCode{}, ErrHandoffInvalid
	}
	if err != nil {
		return HandoffCode{}, err
	}
	c.RedeemedAt = now
	if inTx != nil {
		if err := inTx(ctx, c); err != nil {
			return HandoffCode{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return HandoffCode{}, err
	}
	return c, nil
}

func (r *PostgresHandoffRepository) PeekHandoffCode(ctx context.Context, codeHash []byte, targetApp string, now time.Time) (HandoffCode, error) {
	var c HandoffCode
	err := r.pool.QueryRow(ctx, `SELECT code_hash, user_id, source_sid, target_app, expires_at, created_at
		FROM handoff_codes
		WHERE code_hash = $1 AND target_app = $2 AND redeemed_at IS NULL AND expires_at >= $3`,
		codeHash, targetApp, now).Scan(&c.CodeHash, &c.UserID, &c.SourceSID, &c.TargetApp, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffCode{}, ErrHandoffInvalid
	}
	if err != nil {
		return HandoffCode{}, err
	}
	return c, nil
}

func (r *PostgresHandoffRepository) CountLiveHandoffCodes(ctx context.Context, userID string, now time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM handoff_codes
		WHERE user_id = $1 AND redeemed_at IS NULL AND expires_at >= $2`, userID, now).Scan(&n)
	return n, err
}

func (r *PostgresHandoffRepository) PurgeHandoffCodes(ctx context.Context, now time.Time, limit int) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM handoff_codes WHERE ctid IN (
		SELECT ctid FROM handoff_codes WHERE expires_at < $1 LIMIT $2)`, now.Add(-handoffPurgeRetention), limit)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
