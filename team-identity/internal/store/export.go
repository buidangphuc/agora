package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const exportRowCap = 1000

type ExportAccount struct {
	UserID                string
	Username              string
	Roles                 []string
	Status                string
	CreatedAt             time.Time
	SellerTermsVersion    string
	SellerTermsAcceptedAt *time.Time
	DeletionRequestedAt   *time.Time
	EraseAfter            *time.Time
}

type UserExport struct {
	Account               ExportAccount
	Addresses             []Address
	Sessions              []Session
	LoginHistory          []LoginEvent
	Consents              []Consent
	SessionsTruncated     bool
	LoginHistoryTruncated bool
}

type PostgresExportRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresExportRepository(pool *pgxpool.Pool) *PostgresExportRepository {
	return &PostgresExportRepository{pool: pool}
}

func (r *PostgresExportRepository) ExportUserData(ctx context.Context, userID string) (UserExport, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return UserExport{}, fmt.Errorf("export begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := exportRead(ctx, tx, userID)
	if err != nil {
		return UserExport{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserExport{}, fmt.Errorf("export commit: %w", err)
	}
	return out, nil
}

func exportRead(ctx context.Context, tx pgx.Tx, userID string) (UserExport, error) {
	out := UserExport{
		Addresses:    []Address{},
		Sessions:     []Session{},
		LoginHistory: []LoginEvent{},
		Consents:     []Consent{},
	}
	a := &out.Account
	err := tx.QueryRow(ctx, `SELECT id, username, roles, status, created_at,
		COALESCE(seller_terms_version, ''), seller_terms_accepted_at, deletion_requested_at, erase_after
		FROM users WHERE id = $1`, userID).Scan(&a.UserID, &a.Username, &a.Roles, &a.Status, &a.CreatedAt,
		&a.SellerTermsVersion, &a.SellerTermsAcceptedAt, &a.DeletionRequestedAt, &a.EraseAfter)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserExport{}, ErrNotFound
		}
		return UserExport{}, fmt.Errorf("export account: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT `+addressColumns+` FROM user_addresses WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC, id`, userID)
	if err != nil {
		return UserExport{}, fmt.Errorf("export addresses: %w", err)
	}
	for rows.Next() {
		var ad Address
		if err := scanAddress(rows, &ad); err != nil {
			rows.Close()
			return UserExport{}, fmt.Errorf("export addresses: %w", err)
		}
		out.Addresses = append(out.Addresses, ad)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return UserExport{}, fmt.Errorf("export addresses: %w", err)
	}

	rows, err = tx.Query(ctx, `SELECT id, user_id, device, ip, created_at, last_seen, revoked FROM sessions
		WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, exportRowCap+1)
	if err != nil {
		return UserExport{}, fmt.Errorf("export sessions: %w", err)
	}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Device, &s.IP, &s.CreatedAt, &s.LastSeen, &s.Revoked); err != nil {
			rows.Close()
			return UserExport{}, fmt.Errorf("export sessions: %w", err)
		}
		out.Sessions = append(out.Sessions, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return UserExport{}, fmt.Errorf("export sessions: %w", err)
	}
	if len(out.Sessions) > exportRowCap {
		out.Sessions = out.Sessions[:exportRowCap]
		out.SessionsTruncated = true
	}

	rows, err = tx.Query(ctx, `SELECT id, user_id, ip, user_agent, success, created_at FROM login_history
		WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, exportRowCap+1)
	if err != nil {
		return UserExport{}, fmt.Errorf("export login history: %w", err)
	}
	for rows.Next() {
		var e LoginEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.IP, &e.UserAgent, &e.Success, &e.CreatedAt); err != nil {
			rows.Close()
			return UserExport{}, fmt.Errorf("export login history: %w", err)
		}
		out.LoginHistory = append(out.LoginHistory, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return UserExport{}, fmt.Errorf("export login history: %w", err)
	}
	if len(out.LoginHistory) > exportRowCap {
		out.LoginHistory = out.LoginHistory[:exportRowCap]
		out.LoginHistoryTruncated = true
	}

	rows, err = tx.Query(ctx, `SELECT id, user_id, purpose, granted, version, source, recorded_at FROM consents
		WHERE user_id = $1 ORDER BY recorded_at DESC, id DESC`, userID)
	if err != nil {
		return UserExport{}, fmt.Errorf("export consents: %w", err)
	}
	for rows.Next() {
		var c Consent
		if err := rows.Scan(&c.ID, &c.UserID, &c.Purpose, &c.Granted, &c.Version, &c.Source, &c.RecordedAt); err != nil {
			rows.Close()
			return UserExport{}, fmt.Errorf("export consents: %w", err)
		}
		out.Consents = append(out.Consents, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return UserExport{}, fmt.Errorf("export consents: %w", err)
	}
	return out, nil
}
