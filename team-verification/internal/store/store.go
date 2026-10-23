package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status is the KYC lifecycle state, persisted as a string in {PENDING,
// VERIFIED, REJECTED} (matches the kyc_submissions.status column).
type Status string

const (
	StatusPending  Status = "PENDING"
	StatusVerified Status = "VERIFIED"
	StatusRejected Status = "REJECTED"
)

// ErrNotFound is returned when no submission matches the given id.
var ErrNotFound = errors.New("kyc submission not found")

// ErrAlreadyReviewed is returned when a review targets a submission that is no
// longer PENDING: a decision is final and concurrent reviews cannot overwrite it.
var ErrAlreadyReviewed = errors.New("kyc submission already reviewed")

// Submission is one KYC record. reviewed_at is nil until a reviewer acts.
type Submission struct {
	ID         string
	UserID     string
	DocType    string
	DocRef     string
	Status     Status
	ReviewedAt *time.Time
	CreatedAt  time.Time
}

// KycRepository stores KYC submissions. One user may submit multiple times;
// GetLatestByUser returns the most recent for status lookups. Owned by
// team-verification.
type KycRepository interface {
	// Create persists a new submission (id pre-assigned by the caller/service).
	Create(ctx context.Context, s *Submission) (*Submission, error)
	// GetByID returns a submission by id, or ErrNotFound.
	GetByID(ctx context.Context, id string) (*Submission, error)
	// GetLatestByUser returns the user's most recent submission, or ErrNotFound
	// when the user has never submitted.
	GetLatestByUser(ctx context.Context, userID string) (*Submission, error)
	// UpdateStatus sets the status (and reviewed_at) of a PENDING submission,
	// returning the stored value; ErrAlreadyReviewed if it is no longer PENDING,
	// ErrNotFound if it does not exist.
	UpdateStatus(ctx context.Context, id string, status Status, reviewedAt time.Time) (*Submission, error)
}

// ── Postgres implementation ──

type PostgresKycRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresKycRepo(pool *pgxpool.Pool) *PostgresKycRepo {
	return &PostgresKycRepo{pool: pool}
}

func (r *PostgresKycRepo) Create(ctx context.Context, s *Submission) (*Submission, error) {
	const q = `
		INSERT INTO kyc_submissions (id, user_id, doc_type, doc_ref, status, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING created_at`
	row := r.pool.QueryRow(ctx, q, s.ID, s.UserID, s.DocType, s.DocRef, string(s.Status))
	if err := row.Scan(&s.CreatedAt); err != nil {
		return nil, err
	}
	return clone(s), nil
}

func (r *PostgresKycRepo) GetByID(ctx context.Context, id string) (*Submission, error) {
	return r.queryOne(ctx, `
		SELECT id, user_id, doc_type, doc_ref, status, reviewed_at, created_at
		FROM kyc_submissions
		WHERE id = $1`, id)
}

func (r *PostgresKycRepo) GetLatestByUser(ctx context.Context, userID string) (*Submission, error) {
	return r.queryOne(ctx, `
		SELECT id, user_id, doc_type, doc_ref, status, reviewed_at, created_at
		FROM kyc_submissions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, userID)
}

func (r *PostgresKycRepo) UpdateStatus(ctx context.Context, id string, status Status, reviewedAt time.Time) (*Submission, error) {
	// The PENDING guard makes the decision final and concurrent reviews safe: only
	// one UPDATE can match, the other sees no row.
	const q = `
		UPDATE kyc_submissions
		SET status = $2, reviewed_at = $3
		WHERE id = $1 AND status = $4
		RETURNING id, user_id, doc_type, doc_ref, status, reviewed_at, created_at`
	sub, err := r.scanRow(r.pool.QueryRow(ctx, q, id, string(status), reviewedAt, string(StatusPending)))
	if !errors.Is(err, ErrNotFound) {
		return sub, err
	}
	if _, getErr := r.GetByID(ctx, id); getErr == nil {
		return nil, ErrAlreadyReviewed
	}
	return nil, ErrNotFound
}

func (r *PostgresKycRepo) queryOne(ctx context.Context, q, arg string) (*Submission, error) {
	return r.scanRow(r.pool.QueryRow(ctx, q, arg))
}

func (r *PostgresKycRepo) scanRow(row pgx.Row) (*Submission, error) {
	var s Submission
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.DocType, &s.DocRef, &status, &s.ReviewedAt, &s.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.Status = Status(status)
	return &s, nil
}

func clone(s *Submission) *Submission {
	out := *s
	if s.ReviewedAt != nil {
		rt := *s.ReviewedAt
		out.ReviewedAt = &rt
	}
	return &out
}
