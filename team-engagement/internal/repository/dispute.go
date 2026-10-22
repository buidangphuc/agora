package repository

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

var (
	ErrDisputeNotFound   = errors.New("dispute not found")
	ErrOpenDisputeExists = errors.New("an open dispute already exists for this return")
	ErrDisputeNotInState = errors.New("dispute is not in the expected status")
)

const (
	defaultDisputePageSize = 20
	maxDisputePageSize     = 100
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
	ReturnID     string
	Ruling       engagementv1.DisputeRuling
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DisputeRepository interface {
	CreateDispute(ctx context.Context, d Dispute) (Dispute, error)
	GetDispute(ctx context.Context, id string) (Dispute, error)
	UpdateDispute(ctx context.Context, d Dispute) (Dispute, error)
	ResolveWithEvent(ctx context.Context, d Dispute, prev DisputeStatus, fact proto.Message) (Dispute, error)
	ListByOrder(ctx context.Context, orderID string) ([]Dispute, error)
	ListByStatus(ctx context.Context, status DisputeStatus, pageSize int, cursor string) ([]Dispute, string, error)
}

func clampDisputePageSize(n int) int {
	if n <= 0 {
		return defaultDisputePageSize
	}
	if n > maxDisputePageSize {
		return maxDisputePageSize
	}
	return n
}

func encodeDisputeCursor(d Dispute) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", d.CreatedAt.UnixMicro(), d.ID)))
}

func decodeDisputeCursor(c string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor: %w", err)
	}
	parts := strings.SplitN(string(raw), "|", 2)
	var us int64
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("invalid cursor")
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &us); err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor: %w", err)
	}
	return time.UnixMicro(us).UTC(), parts[1], nil
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresDisputeRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresDisputeRepository(pool *pgxpool.Pool) *PostgresDisputeRepository {
	return &PostgresDisputeRepository{pool: pool}
}

const disputeColumns = `id, order_id, claimant_id, defendant_id, reason, evidence_urls, status, resolution, return_id, ruling, created_at, updated_at`

type disputeScanner interface {
	Scan(dest ...any) error
}

func scanDispute(s disputeScanner) (Dispute, error) {
	var d Dispute
	var statusStr string
	var ruling int32
	if err := s.Scan(&d.ID, &d.OrderID, &d.ClaimantID, &d.DefendantID, &d.Reason, &d.EvidenceURLs, &statusStr, &d.Resolution, &d.ReturnID, &ruling, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return Dispute{}, err
	}
	d.Status = DisputeStatus(statusStr)
	d.Ruling = engagementv1.DisputeRuling(ruling)
	if d.EvidenceURLs == nil {
		d.EvidenceURLs = make([]string, 0)
	}
	return d, nil
}

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

	const query = `INSERT INTO disputes (id, order_id, claimant_id, defendant_id, reason, evidence_urls, status, resolution, return_id, ruling, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	if _, err := r.pool.Exec(ctx, query, d.ID, d.OrderID, d.ClaimantID, d.DefendantID, d.Reason, d.EvidenceURLs, string(d.Status), d.Resolution, d.ReturnID, int32(d.Ruling), d.CreatedAt, d.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_disputes_open_return" {
			return Dispute{}, ErrOpenDisputeExists
		}
		return Dispute{}, fmt.Errorf("insert dispute: %w", err)
	}
	return d, nil
}

func (r *PostgresDisputeRepository) GetDispute(ctx context.Context, id string) (Dispute, error) {
	const query = `SELECT ` + disputeColumns + ` FROM disputes WHERE id = $1`
	d, err := scanDispute(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dispute{}, ErrDisputeNotFound
		}
		return Dispute{}, fmt.Errorf("get dispute: %w", err)
	}
	return d, nil
}

func (r *PostgresDisputeRepository) UpdateDispute(ctx context.Context, d Dispute) (Dispute, error) {
	d.UpdatedAt = time.Now().UTC()
	const query = `UPDATE disputes SET status = $1, resolution = $2, ruling = $3, updated_at = $4 WHERE id = $5
		RETURNING ` + disputeColumns

	updated, err := scanDispute(r.pool.QueryRow(ctx, query, string(d.Status), d.Resolution, int32(d.Ruling), d.UpdatedAt, d.ID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Dispute{}, ErrDisputeNotFound
		}
		return Dispute{}, fmt.Errorf("update dispute: %w", err)
	}
	return updated, nil
}

func (r *PostgresDisputeRepository) ResolveWithEvent(ctx context.Context, d Dispute, prev DisputeStatus, fact proto.Message) (Dispute, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Dispute{}, fmt.Errorf("begin resolve dispute: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `UPDATE disputes SET status = $1, resolution = $2, ruling = $3, updated_at = $4 WHERE id = $5 AND status = $6
		RETURNING ` + disputeColumns
	updated, err := scanDispute(tx.QueryRow(ctx, query, string(d.Status), d.Resolution, int32(d.Ruling), time.Now().UTC(), d.ID, string(prev)))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return Dispute{}, fmt.Errorf("resolve dispute: %w", err)
		}
		var exists bool
		if qerr := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM disputes WHERE id = $1)`, d.ID).Scan(&exists); qerr != nil {
			return Dispute{}, fmt.Errorf("resolve dispute: %w", qerr)
		}
		if !exists {
			return Dispute{}, ErrDisputeNotFound
		}
		return Dispute{}, ErrDisputeNotInState
	}
	if fact != nil {
		if err := enqueueFact(ctx, tx, updated.ID, fact); err != nil {
			return Dispute{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Dispute{}, fmt.Errorf("commit resolve dispute: %w", err)
	}
	return updated, nil
}

func (r *PostgresDisputeRepository) ListByOrder(ctx context.Context, orderID string) ([]Dispute, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+disputeColumns+` FROM disputes WHERE order_id = $1 ORDER BY created_at DESC, id DESC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list disputes by order: %w", err)
	}
	defer rows.Close()
	out := make([]Dispute, 0)
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, fmt.Errorf("scan dispute: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list disputes by order: %w", err)
	}
	return out, nil
}

func (r *PostgresDisputeRepository) ListByStatus(ctx context.Context, status DisputeStatus, pageSize int, cursor string) ([]Dispute, string, error) {
	size := clampDisputePageSize(pageSize)
	var curAt *time.Time
	curID := ""
	if cursor != "" {
		at, id, err := decodeDisputeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		curAt, curID = &at, id
	}
	const query = `SELECT ` + disputeColumns + ` FROM disputes
		WHERE ($1 = '' OR status = $1)
		  AND ($2::timestamptz IS NULL OR (created_at, id) > ($2, $3))
		ORDER BY created_at, id LIMIT $4`
	rows, err := r.pool.Query(ctx, query, string(status), curAt, curID, size+1)
	if err != nil {
		return nil, "", fmt.Errorf("list disputes: %w", err)
	}
	defer rows.Close()
	out := make([]Dispute, 0, size+1)
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, "", fmt.Errorf("scan dispute: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("list disputes: %w", err)
	}
	next := ""
	if len(out) > size {
		out = out[:size]
		next = encodeDisputeCursor(out[size-1])
	}
	return out, next, nil
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

func disputeIsOpen(s DisputeStatus) bool {
	return s == DisputeStatusOpen || s == DisputeStatusInvestigating
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
	if d.ReturnID != "" && disputeIsOpen(d.Status) {
		for _, e := range r.data {
			if e.ReturnID == d.ReturnID && disputeIsOpen(e.Status) {
				return Dispute{}, ErrOpenDisputeExists
			}
		}
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
	existing.Ruling = d.Ruling
	existing.UpdatedAt = time.Now().UTC()
	r.data[d.ID] = existing
	return existing, nil
}

func (r *InMemoryDisputeRepository) ResolveWithEvent(_ context.Context, d Dispute, prev DisputeStatus, _ proto.Message) (Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[d.ID]
	if !ok {
		return Dispute{}, ErrDisputeNotFound
	}
	if existing.Status != prev {
		return Dispute{}, ErrDisputeNotInState
	}
	existing.Status = d.Status
	existing.Resolution = d.Resolution
	existing.Ruling = d.Ruling
	existing.UpdatedAt = time.Now().UTC()
	r.data[d.ID] = existing
	return existing, nil
}

func (r *InMemoryDisputeRepository) ListByOrder(_ context.Context, orderID string) ([]Dispute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Dispute, 0)
	for _, d := range r.data {
		if d.OrderID == orderID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (r *InMemoryDisputeRepository) ListByStatus(_ context.Context, status DisputeStatus, pageSize int, cursor string) ([]Dispute, string, error) {
	size := clampDisputePageSize(pageSize)
	var curAt time.Time
	curID := ""
	if cursor != "" {
		at, id, err := decodeDisputeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		curAt, curID = at, id
	}

	r.mu.RLock()
	all := make([]Dispute, 0, len(r.data))
	for _, d := range r.data {
		if status == "" || d.Status == status {
			all = append(all, d)
		}
	}
	r.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})

	out := make([]Dispute, 0, size+1)
	for _, d := range all {
		if cursor != "" {
			at := d.CreatedAt.Truncate(time.Microsecond)
			if at.Before(curAt) || (at.Equal(curAt) && d.ID <= curID) {
				continue
			}
		}
		out = append(out, d)
		if len(out) > size {
			break
		}
	}
	next := ""
	if len(out) > size {
		out = out[:size]
		next = encodeDisputeCursor(out[size-1])
	}
	return out, next, nil
}

var (
	_ DisputeRepository = (*PostgresDisputeRepository)(nil)
	_ DisputeRepository = (*InMemoryDisputeRepository)(nil)
)
