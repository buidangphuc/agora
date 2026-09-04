package repository

import (
	"context"
	"errors"
	"fmt"

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

const userColumns = `id, username, password_hash, roles`

// Create inserts a user, mapping a unique-violation to ErrConflict.
func (r *PostgresUserRepository) Create(ctx context.Context, u User) (User, error) {
	const q = `INSERT INTO users (id, username, password_hash, roles)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + userColumns
	var out User
	err := r.pool.QueryRow(ctx, q, u.ID, u.Username, u.PasswordHash, u.Roles).Scan(
		&out.ID, &out.Username, &out.PasswordHash, &out.Roles,
	)
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
	err := r.pool.QueryRow(ctx, q, username).Scan(
		&out.ID, &out.Username, &out.PasswordHash, &out.Roles,
	)
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
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&out.ID, &out.Username, &out.PasswordHash, &out.Roles,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by id %q: %w", id, err)
	}
	return out, nil
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

var _ UserRepository = (*PostgresUserRepository)(nil)
