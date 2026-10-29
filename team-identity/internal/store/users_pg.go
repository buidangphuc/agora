package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresUserRepository is the production store (identity_db, Rule 3).
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

const userColumns = `id, username, password_hash, roles, status, COALESCE(status_reason, ''), status_until, status_changed_at, created_at`

func scanUser(row pgx.Row, u *User) error {
	var until, changed *time.Time
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Roles, &u.Status, &u.StatusReason, &until, &changed, &u.CreatedAt); err != nil {
		return err
	}
	u.StatusUntil, u.StatusChangedAt = time.Time{}, time.Time{}
	if until != nil {
		u.StatusUntil = *until
	}
	if changed != nil {
		u.StatusChangedAt = *changed
	}
	return nil
}

// Create inserts a user, mapping a unique-violation to ErrConflict.
func (r *PostgresUserRepository) Create(ctx context.Context, u User) (User, error) {
	const q = `INSERT INTO users (id, username, password_hash, roles)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + userColumns
	var out User
	err := scanUser(r.pool.QueryRow(ctx, q, u.ID, u.Username, u.PasswordHash, u.Roles), &out)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return User{}, ErrConflict
		}
		return User{}, fmt.Errorf("create user %q: %w", u.Username, err)
	}
	return out, nil
}

// GetByUsername loads a user by username, mapping a miss to ErrNotFound.
func (r *PostgresUserRepository) GetByUsername(ctx context.Context, username string) (User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE username = $1`
	var out User
	err := scanUser(r.pool.QueryRow(ctx, q, username), &out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user %q: %w", username, err)
	}
	return out, nil
}

// GetByID loads a user by id, mapping a miss to ErrNotFound.
func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	var out User
	err := scanUser(r.pool.QueryRow(ctx, q, id), &out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by id %q: %w", id, err)
	}
	return out, nil
}

// GetByIDs loads the users with the given ids in one query. Unknown ids are
// omitted. Only id and username are selected: this backs the public-profile
// lookup and must not read credentials.
func (r *PostgresUserRepository) GetByIDs(ctx context.Context, ids []string) ([]User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `SELECT id, username FROM users WHERE id = ANY($1)`
	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("get users by ids: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdatePassword updates user password hash.
func (r *PostgresUserRepository) UpdatePassword(ctx context.Context, userID, newPasswordHash string) error {
	const q = `UPDATE users SET password_hash = $2 WHERE id = $1`
	res, err := r.pool.Exec(ctx, q, userID, newPasswordHash)
	if err != nil {
		return fmt.Errorf("update password for %q: %w", userID, err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateResetToken inserts a password reset token.
func (r *PostgresUserRepository) CreateResetToken(ctx context.Context, token PasswordResetToken) error {
	const q = `INSERT INTO password_reset_tokens (token_hash, user_id, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, '0001-01-01 00:00:00+00'::timestamptz), now()))`
	_, err := r.pool.Exec(ctx, q, token.TokenHash, token.UserID, token.ExpiresAt, token.Used, token.CreatedAt)
	if err != nil {
		return fmt.Errorf("create reset token: %w", err)
	}
	return nil
}

// GetResetToken retrieves a password reset token by its hash.
func (r *PostgresUserRepository) GetResetToken(ctx context.Context, tokenHash string) (PasswordResetToken, error) {
	const q = `SELECT token_hash, user_id, expires_at, used, created_at FROM password_reset_tokens WHERE token_hash = $1`
	var out PasswordResetToken
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(
		&out.TokenHash, &out.UserID, &out.ExpiresAt, &out.Used, &out.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PasswordResetToken{}, ErrTokenNotFound
		}
		return PasswordResetToken{}, fmt.Errorf("get reset token: %w", err)
	}
	return out, nil
}

// MarkResetTokenUsed marks a reset token as used.
func (r *PostgresUserRepository) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	const q = `UPDATE password_reset_tokens SET used = true WHERE token_hash = $1`
	res, err := r.pool.Exec(ctx, q, tokenHash)
	if err != nil {
		return fmt.Errorf("mark reset token used: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// ListUsers matches id exactly or username by literal prefix (starts_with, so LIKE
// wildcards in the query are inert), ordered by username in byte order. The
// cursor is the last username of the previous page.
func (r *PostgresUserRepository) ListUsers(ctx context.Context, query, status, cursor string, limit int) ([]User, string, error) {
	limit = clampListLimit(limit)
	const q = `SELECT ` + userColumns + ` FROM users
		WHERE ($1 = '' OR id = $1 OR starts_with(username, $1))
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR username COLLATE "C" > $3 COLLATE "C")
		ORDER BY username COLLATE "C"
		LIMIT $4`
	rows, err := r.pool.Query(ctx, q, query, status, cursor, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := scanUser(rows, &u); err != nil {
			return nil, "", fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(out) > limit {
		return out[:limit], out[limit-1].Username, nil
	}
	return out, "", nil
}

// SetStatus changes the user's status in one transaction: lock the user row, revoke
// every live session (non-active targets) with one revoke event each, update the
// user, then enqueue the status event last.
func (r *PostgresUserRepository) SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke EnqueueFn, changed StatusEnqueueFn) (StatusResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return StatusResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var prev string
	if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&prev); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StatusResult{}, ErrNotFound
		}
		return StatusResult{}, fmt.Errorf("lock user %q: %w", userID, err)
	}

	revoked := 0
	if status != StatusActive {
		const rev = `UPDATE sessions SET revoked = true, revoked_at = now(), refresh_hash = NULL, prev_refresh_hash = NULL
			WHERE user_id = $1 AND NOT revoked RETURNING ` + sessionColumns
		rows, err := tx.Query(ctx, rev, userID)
		if err != nil {
			return StatusResult{}, fmt.Errorf("revoke sessions: %w", err)
		}
		var sessions []Session
		for rows.Next() {
			var s Session
			if err := scanSession(rows, &s); err != nil {
				rows.Close()
				return StatusResult{}, fmt.Errorf("scan session: %w", err)
			}
			sessions = append(sessions, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return StatusResult{}, fmt.Errorf("revoke sessions: %w", err)
		}
		sort.Slice(sessions, func(i, j int) bool {
			if !sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
				return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
			}
			return sessions[i].ID < sessions[j].ID
		})
		for _, s := range sessions {
			row, err := revoke(s)
			if err != nil {
				return StatusResult{}, fmt.Errorf("build revoke event: %w", err)
			}
			if err := enqueueTx(ctx, tx, row); err != nil {
				return StatusResult{}, err
			}
			revoked++
		}
	}

	const upd = `UPDATE users SET status = $2, status_reason = NULLIF($3, ''), status_until = $4, status_changed_at = $5
		WHERE id = $1 RETURNING ` + userColumns
	var u User
	if err := scanUser(tx.QueryRow(ctx, upd, userID, status, reason, nullTime(until), now), &u); err != nil {
		return StatusResult{}, fmt.Errorf("update user status: %w", err)
	}
	row, err := changed(StatusChange{UserID: userID, PreviousStatus: prev, Status: status, Reason: reason, Until: until, ChangedAt: now})
	if err != nil {
		return StatusResult{}, fmt.Errorf("build status event: %w", err)
	}
	if err := enqueueTx(ctx, tx, row); err != nil {
		return StatusResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StatusResult{}, fmt.Errorf("commit tx: %w", err)
	}
	return StatusResult{User: u, SessionsRevoked: revoked}, nil
}

// LiftExpiredSuspension atomically returns an expired suspension to active. The
// conditional UPDATE serialises concurrent callers: only one sees the row.
func (r *PostgresUserRepository) LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed StatusEnqueueFn) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	const upd = `UPDATE users SET status = 'active', status_reason = NULL, status_until = NULL, status_changed_at = $2
		WHERE id = $1 AND status = 'suspended' AND status_until IS NOT NULL AND status_until <= $2
		RETURNING id`
	var id string
	if err := tx.QueryRow(ctx, upd, userID, now).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("lift suspension: %w", err)
	}
	row, err := changed(StatusChange{UserID: userID, PreviousStatus: StatusSuspended, Status: StatusActive, ChangedAt: now})
	if err != nil {
		return false, fmt.Errorf("build status event: %w", err)
	}
	if err := enqueueTx(ctx, tx, row); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

var _ UserRepository = (*PostgresUserRepository)(nil)
