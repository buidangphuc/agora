package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const StatusDeleted = "deleted"

var ErrNoPendingDeletion = errors.New("no pending account deletion")

type DeletionSchedule struct {
	RequestedAt     time.Time
	EraseAfter      time.Time
	Created         bool
	SessionsRevoked int
}

type Erasure struct {
	UserID   string
	Roles    []string
	ErasedAt time.Time
}

type ErasedEnqueueFn func(Erasure) (OutboxRow, error)

type ErasureResult struct {
	Erased  int
	Failed  int
	Skipped int
}

func (r *PostgresUserRepository) ScheduleDeletion(ctx context.Context, userID, keepSessionID string, now time.Time, window time.Duration) (DeletionSchedule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DeletionSchedule{}, fmt.Errorf("begin schedule deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var requested, erase *time.Time
	err = tx.QueryRow(ctx, `SELECT deletion_requested_at, erase_after FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&requested, &erase)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DeletionSchedule{}, ErrNotFound
		}
		return DeletionSchedule{}, fmt.Errorf("lock user for deletion: %w", err)
	}
	if requested != nil && erase != nil {
		return DeletionSchedule{RequestedAt: *requested, EraseAfter: *erase}, nil
	}

	eraseAfter := now.Add(window)
	if _, err := tx.Exec(ctx, `UPDATE users SET deletion_requested_at = $2, erase_after = $3 WHERE id = $1`, userID, now, eraseAfter); err != nil {
		return DeletionSchedule{}, fmt.Errorf("schedule deletion: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked = true, revoked_at = now(), refresh_hash = NULL, prev_refresh_hash = NULL
		WHERE user_id = $1 AND id <> $2 AND NOT revoked`, userID, keepSessionID)
	if err != nil {
		return DeletionSchedule{}, fmt.Errorf("revoke other sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletionSchedule{}, fmt.Errorf("commit schedule deletion: %w", err)
	}
	return DeletionSchedule{RequestedAt: now, EraseAfter: eraseAfter, Created: true, SessionsRevoked: int(tag.RowsAffected())}, nil
}

func (r *PostgresUserRepository) CancelDeletion(ctx context.Context, userID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET deletion_requested_at = NULL, erase_after = NULL
		WHERE id = $1 AND erase_after IS NOT NULL AND status <> 'deleted'`, userID)
	if err != nil {
		return fmt.Errorf("cancel deletion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoPendingDeletion
	}
	return nil
}

func (r *PostgresUserRepository) EraseDue(ctx context.Context, now time.Time, limit int, enqueue ErasedEnqueueFn) (ErasureResult, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM users
		WHERE erase_after IS NOT NULL AND erase_after <= $1 AND status <> 'deleted'
		ORDER BY erase_after, id LIMIT $2`, now, limit)
	if err != nil {
		return ErasureResult{}, fmt.Errorf("list due erasures: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ErasureResult{}, fmt.Errorf("scan due erasure: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ErasureResult{}, fmt.Errorf("list due erasures: %w", err)
	}

	var res ErasureResult
	for _, id := range ids {
		erased, err := r.eraseOne(ctx, id, now, enqueue)
		switch {
		case err != nil:
			res.Failed++
		case erased:
			res.Erased++
		default:
			res.Skipped++
		}
	}
	return res, nil
}

func (r *PostgresUserRepository) eraseOne(ctx context.Context, id string, now time.Time, enqueue ErasedEnqueueFn) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var roles []string
	err = tx.QueryRow(ctx, `SELECT roles FROM users
		WHERE id = $1 AND erase_after IS NOT NULL AND erase_after <= $2 AND status <> 'deleted'
		FOR UPDATE SKIP LOCKED`, id, now).Scan(&roles)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	for _, tbl := range []string{"user_addresses", "sessions", "login_history", "handoff_codes", "password_reset_tokens"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+tbl+` WHERE user_id = $1`, id); err != nil {
			return false, fmt.Errorf("erase %s: %w", tbl, err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET
			username = 'deleted_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 16),
			password_hash = '', status = 'deleted', roles = '{}',
			status_reason = NULL, status_until = NULL,
			deletion_requested_at = NULL, erase_after = NULL
		WHERE id = $1`, id); err != nil {
		return false, fmt.Errorf("anonymise user: %w", err)
	}
	row, err := enqueue(Erasure{UserID: id, Roles: roles, ErasedAt: now})
	if err != nil {
		return false, err
	}
	if err := enqueueTx(ctx, r.ob, tx, row); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
