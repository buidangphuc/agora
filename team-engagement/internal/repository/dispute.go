package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDisputeNotFound = errors.New("dispute not found")
)

type DisputeStatus string

const (
	DisputeStatusOpen          DisputeStatus = "OPEN"
	DisputeStatusInvestigating DisputeStatus = "INVESTIGATING"
	DisputeStatusResolved      DisputeStatus = "RESOLVED"
	DisputeStatusRejected      DisputeStatus = "REJECTED"
)

type Dispute struct {
	ID           string
	OrderID      string
	ClaimantID   string
	DefendantID  string
	Reason       string
	EvidenceURLs []string
	Status       DisputeStatus
	Resolution   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DisputeRepository interface {
	CreateDispute(ctx context.Context, d Dispute) (Dispute, error)
	GetDispute(ctx context.Context, id string) (Dispute, error)
	UpdateDispute(ctx context.Context, d Dispute) (Dispute, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresDisputeRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresDisputeRepository(pool *pgxpool.Pool) *PostgresDisputeRepository {
	return &PostgresDisputeRepository{pool: pool}
}

const disputeColumns = `id, order_id, claimant_id, defendant_id, reason, evidence_urls, status, resolution, created_at, updated_at`

func (r *PostgresDisputeRepository) CreateDispute(ctx context.Context, d Dispute) (Dispute, error) {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Status == "" {
		d.Status = DisputeStatusOpen
	}
	if d.EvidenceURLs == nil {
		d.EvidenceURLs = make([]string, 0)
	}

	const query = `INSERT INTO disputes (id, order_id, claimant_id, defendant_id, reason, evidence_urls, status, resolution, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	if _, err := r.pool.Exec(ctx, query, d.ID, d.OrderID, d.ClaimantID, d.DefendantID, d.Reason, d.EvidenceURLs, string(d.Status), d.Resolution, d.CreatedAt, d.UpdatedAt); err != nil {
		return Dispute{}, fmt.Errorf("insert dispute: %w", err)
	}
	return d, nil
}

func (r *PostgresDisputeRepository) GetDispute(ctx context.Context, id string) (Dispute, error) {
	var d Dispute
	var statusStr string
	const query = `SELECT ` + disputeColumns + ` FROM disputes WHERE id = $1`
	if err := r.pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.OrderID, &d.ClaimantID, &d.DefendantID, &d.Reason, &d.EvidenceURLs, &statusStr, &d.Resolution, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dispute{}, ErrDisputeNotFound
		}
		return Dispute{}, fmt.Errorf("get dispute: %w", err)
	}
	d.Status = DisputeStatus(statusStr)
	if d.EvidenceURLs == nil {
		d.EvidenceURLs = make([]string, 0)
	}
	return d, nil
}

func (r *PostgresDisputeRepository) UpdateDispute(ctx context.Context, d Dispute) (Dispute, error) {
	d.UpdatedAt = time.Now().UTC()
	const query = `UPDATE disputes SET status = $1, resolution = $2, updated_at = $3 WHERE id = $4
		RETURNING id, order_id, claimant_id, defendant_id, reason, evidence_urls, status, resolution, created_at, updated_at`

	var updated Dispute
	var statusStr string
	if err := r.pool.QueryRow(ctx, query, string(d.Status), d.Resolution, d.UpdatedAt, d.ID).Scan(
		&updated.ID, &updated.OrderID, &updated.ClaimantID, &updated.DefendantID, &updated.Reason, &updated.EvidenceURLs, &statusStr, &updated.Resolution, &updated.CreatedAt, &updated.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dispute{}, ErrDisputeNotFound
		}
		return Dispute{}, fmt.Errorf("update dispute: %w", err)
	}
	updated.Status = DisputeStatus(statusStr)
	if updated.EvidenceURLs == nil {
		updated.EvidenceURLs = make([]string, 0)
	}
	return updated, nil
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryDisputeRepository struct {
	mu   sync.RWMutex
	data map[string]Dispute
}

func NewInMemoryDisputeRepository() *InMemoryDisputeRepository {
	return &InMemoryDisputeRepository{
		data: make(map[string]Dispute),
	}
}

func (r *InMemoryDisputeRepository) CreateDispute(_ context.Context, d Dispute) (Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Status == "" {
		d.Status = DisputeStatusOpen
	}
	if d.EvidenceURLs == nil {
		d.EvidenceURLs = make([]string, 0)
	}

	r.data[d.ID] = d
	return d, nil
}

func (r *InMemoryDisputeRepository) GetDispute(_ context.Context, id string) (Dispute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.data[id]
	if !ok {
		return Dispute{}, ErrDisputeNotFound
	}
	return d, nil
}

func (r *InMemoryDisputeRepository) UpdateDispute(_ context.Context, d Dispute) (Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[d.ID]
	if !ok {
		return Dispute{}, ErrDisputeNotFound
	}

	existing.Status = d.Status
	existing.Resolution = d.Resolution
	existing.UpdatedAt = time.Now().UTC()
	r.data[d.ID] = existing
	return existing, nil
}

var (
	_ DisputeRepository = (*PostgresDisputeRepository)(nil)
	_ DisputeRepository = (*InMemoryDisputeRepository)(nil)
)
