package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrGrantInvalid    = errors.New("upload grant invalid or already used")
	ErrGrantExpired    = errors.New("upload grant expired")
	ErrGrantCapReached = errors.New("too many open upload grants")
)

type Grant struct {
	ObjectKey   string
	UserID      string
	ContentType string
	SizeBytes   int64
	IssuedAt    time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

type Document struct {
	SubmissionID string
	Index        int
	ObjectKey    string
	ContentType  string
	SizeBytes    int64
	DeletedAt    *time.Time
}

type DocumentRepository interface {
	CreateGrant(ctx context.Context, g *Grant, cap int, now time.Time) error
	GetGrant(ctx context.Context, key string) (*Grant, error)
	SubmitWithDocuments(ctx context.Context, s *Submission, keys []string, now time.Time, verify func([]Grant) error) (*Submission, error)
	ListDocuments(ctx context.Context, submissionID string) ([]Document, error)
}

func (r *PostgresKycRepo) CreateGrant(ctx context.Context, g *Grant, cap int, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "kyc_grants:"+g.UserID); err != nil {
		return err
	}
	var open int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM kyc_upload_grants
		WHERE user_id = $1 AND used_at IS NULL AND expires_at > $2`, g.UserID, now).Scan(&open); err != nil {
		return err
	}
	if open >= cap {
		return ErrGrantCapReached
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO kyc_upload_grants (object_key, user_id, content_type, size_bytes, issued_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		g.ObjectKey, g.UserID, g.ContentType, g.SizeBytes, g.IssuedAt, g.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresKycRepo) GetGrant(ctx context.Context, key string) (*Grant, error) {
	var g Grant
	err := r.pool.QueryRow(ctx, `
		SELECT object_key, user_id, content_type, size_bytes, issued_at, expires_at, used_at
		FROM kyc_upload_grants WHERE object_key = $1`, key).
		Scan(&g.ObjectKey, &g.UserID, &g.ContentType, &g.SizeBytes, &g.IssuedAt, &g.ExpiresAt, &g.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *PostgresKycRepo) SubmitWithDocuments(ctx context.Context, s *Submission, keys []string, now time.Time, verify func([]Grant) error) (*Submission, error) {
	if len(keys) == 0 {
		return nil, ErrGrantInvalid
	}
	sorted := slices.Clone(keys)
	slices.Sort(sorted)
	if len(slices.Compact(slices.Clone(sorted))) != len(sorted) {
		return nil, ErrGrantInvalid
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT object_key, user_id, content_type, size_bytes, issued_at, expires_at, used_at
		FROM kyc_upload_grants WHERE object_key = ANY($1)
		ORDER BY object_key FOR UPDATE`, sorted)
	if err != nil {
		return nil, err
	}
	byKey := map[string]Grant{}
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ObjectKey, &g.UserID, &g.ContentType, &g.SizeBytes, &g.IssuedAt, &g.ExpiresAt, &g.UsedAt); err != nil {
			rows.Close()
			return nil, err
		}
		byKey[g.ObjectKey] = g
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	grants := make([]Grant, 0, len(keys))
	expired := false
	for _, k := range keys {
		g, ok := byKey[k]
		if !ok || g.UserID != s.UserID || g.UsedAt != nil {
			return nil, ErrGrantInvalid
		}
		if !g.ExpiresAt.After(now) {
			expired = true
		}
		grants = append(grants, g)
	}
	if expired {
		return nil, ErrGrantExpired
	}
	if verify != nil {
		if err := verify(grants); err != nil {
			return nil, err
		}
	}

	sub := *s
	sub.DocRef = keys[0]
	if err := tx.QueryRow(ctx, `
		INSERT INTO kyc_submissions (id, user_id, doc_type, doc_ref, status, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING created_at`, sub.ID, sub.UserID, sub.DocType, sub.DocRef, string(sub.Status)).Scan(&sub.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE kyc_submissions SET superseded_at = $3
		WHERE user_id = $1 AND id <> $2 AND superseded_at IS NULL`, sub.UserID, sub.ID, now); err != nil {
		return nil, err
	}
	for i, g := range grants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO kyc_documents (submission_id, doc_index, object_key, content_type, size_bytes)
			VALUES ($1, $2, $3, $4, $5)`, sub.ID, i, g.ObjectKey, g.ContentType, g.SizeBytes); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE kyc_upload_grants SET used_at = $2 WHERE object_key = ANY($1)`, sorted, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return clone(&sub), nil
}

func (r *PostgresKycRepo) ListDocuments(ctx context.Context, submissionID string) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT submission_id, doc_index, object_key, content_type, size_bytes, deleted_at
		FROM kyc_documents WHERE submission_id = $1 ORDER BY doc_index`, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.SubmissionID, &d.Index, &d.ObjectKey, &d.ContentType, &d.SizeBytes, &d.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
