package store

import (
	"context"
	"errors"
	"time"
)

type ListFilter struct {
	Status    Status
	HasCursor bool
	AfterAt   time.Time
	AfterID   string
	Limit     int
}

type ReviewRepository interface {
	ReviewWithReason(ctx context.Context, id string, status Status, reasonCode string, reviewedAt time.Time) (*Submission, error)
	ListSubmissions(ctx context.Context, f ListFilter) ([]Submission, error)
	LatestStatuses(ctx context.Context, userIDs []string) (map[string]Status, error)
}

func (r *PostgresKycRepo) ReviewWithReason(ctx context.Context, id string, status Status, reasonCode string, reviewedAt time.Time) (*Submission, error) {
	const q = `
		UPDATE kyc_submissions
		SET status = $2, reviewed_at = $3, reason_code = NULLIF($5, '')
		WHERE id = $1 AND status = $4
		RETURNING id, user_id, doc_type, doc_ref, status, reviewed_at, created_at, COALESCE(reason_code, ''), superseded_at`
	sub, err := r.scanRow(r.pool.QueryRow(ctx, q, id, string(status), reviewedAt, string(StatusPending), reasonCode))
	if !errors.Is(err, ErrNotFound) {
		return sub, err
	}
	if _, getErr := r.GetByID(ctx, id); getErr == nil {
		return nil, ErrAlreadyReviewed
	}
	return nil, ErrNotFound
}

func (r *PostgresKycRepo) ListSubmissions(ctx context.Context, f ListFilter) ([]Submission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.user_id, s.doc_type, s.doc_ref, s.status, s.reviewed_at, s.created_at,
		       COALESCE(s.reason_code, ''), s.superseded_at,
		       (SELECT count(*) FROM kyc_documents d WHERE d.submission_id = s.id)
		FROM kyc_submissions s
		WHERE ($1 = '' OR s.status = $1)
		  AND (NOT $2 OR (s.created_at, s.id) > ($3, $4))
		ORDER BY s.created_at, s.id
		LIMIT $5`, string(f.Status), f.HasCursor, f.AfterAt, f.AfterID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Submission
	for rows.Next() {
		var s Submission
		var st string
		if err := rows.Scan(&s.ID, &s.UserID, &s.DocType, &s.DocRef, &st, &s.ReviewedAt, &s.CreatedAt,
			&s.ReasonCode, &s.SupersededAt, &s.DocumentCount); err != nil {
			return nil, err
		}
		s.Status = Status(st)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresKycRepo) LatestStatuses(ctx context.Context, userIDs []string) (map[string]Status, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (user_id) user_id, status
		FROM kyc_submissions
		WHERE user_id = ANY($1)
		ORDER BY user_id, created_at DESC`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Status{}
	for rows.Next() {
		var u, st string
		if err := rows.Scan(&u, &st); err != nil {
			return nil, err
		}
		out[u] = Status(st)
	}
	return out, rows.Err()
}
