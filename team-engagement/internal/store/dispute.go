package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

var (
	ErrDisputeNotFound   = errors.New("dispute not found")
	ErrOpenDisputeExists = errors.New("an open dispute already exists for this return")
	ErrDisputeNotInState = errors.New("dispute is not in the expected status")
)

const (
	DefaultDisputePageSize = 20
	MaxDisputePageSize     = 100
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

func EncodeDisputeCursor(d Dispute) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", d.CreatedAt.UnixMicro(), d.ID)))
}

func DecodeDisputeCursor(c string) (time.Time, string, error) {
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

type PostgresDisputeRepository struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPostgresDisputeRepository(pool *pgxpool.Pool, ob *outbox.Store) *PostgresDisputeRepository {
	return &PostgresDisputeRepository{pool: pool, outbox: ob}
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
	var updated Dispute
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		const query = `UPDATE disputes SET status = $1, resolution = $2, ruling = $3, updated_at = $4 WHERE id = $5 AND status = $6
		RETURNING ` + disputeColumns
		var err error
		updated, err = scanDispute(tx.QueryRow(ctx, query, string(d.Status), d.Resolution, int32(d.Ruling), time.Now().UTC(), d.ID, string(prev)))
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("resolve dispute: %w", err)
			}
			var exists bool
			if qerr := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM disputes WHERE id = $1)`, d.ID).Scan(&exists); qerr != nil {
				return fmt.Errorf("resolve dispute: %w", qerr)
			}
			if !exists {
				return ErrDisputeNotFound
			}
			return ErrDisputeNotInState
		}
		if fact != nil {
			return enqueueFact(ctx, r.outbox, tx, "Dispute", updated.ID, fact)
		}
		return nil
	})
	if err != nil {
		return Dispute{}, err
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
	size := pagination.Clamp(pageSize, DefaultDisputePageSize, MaxDisputePageSize)
	var curAt *time.Time
	curID := ""
	if cursor != "" {
		at, id, err := DecodeDisputeCursor(cursor)
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
		next = EncodeDisputeCursor(out[size-1])
	}
	return out, next, nil
}

var _ DisputeRepository = (*PostgresDisputeRepository)(nil)
