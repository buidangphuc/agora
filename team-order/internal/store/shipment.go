package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

var ErrShipmentNotFound = errors.New("shipment not found")

type ShipmentStatus int32

const (
	ShipmentStatusUnspecified ShipmentStatus = 0
	ShipmentStatusPending     ShipmentStatus = 1
	ShipmentStatusPickedUp    ShipmentStatus = 2
	ShipmentStatusInTransit   ShipmentStatus = 3
	ShipmentStatusDelivered   ShipmentStatus = 4
	ShipmentStatusFailed      ShipmentStatus = 5
)

type ShipmentCheckpoint struct {
	ID          string
	ShipmentID  string
	Timestamp   time.Time
	Location    string
	Description string
	CreatedAt   time.Time
}

type Shipment struct {
	ID           string
	OrderID      string
	Carrier      string
	TrackingCode string
	Status       ShipmentStatus
	Checkpoints  []ShipmentCheckpoint
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// BuyerID and SellerID are NOT persisted on the shipment row. The caller fills
	// them from the order so the OrderShipped outbox row can name the buyer to notify.
	BuyerID  string
	SellerID string
}

// ShipmentOutboxBuilder turns a shipment that is being created into the outbox row
// to persist in the SAME transaction (ADR-0013). Injected by internal/events so the
// repository stays free of an events dependency (same shape as PaidOutboxBuilder).
type ShipmentOutboxBuilder func(s Shipment) (outbox.Row, error)

// ShipmentRepoOption customizes a shipment repository.
type ShipmentRepoOption func(*shipmentRepoConfig)

type shipmentRepoConfig struct {
	shippedOutbox ShipmentOutboxBuilder
}

// WithShipmentOutbox makes every CreateShipment write an outbox row built by b in
// the SAME transaction as the shipment insert. If building or enqueueing the row
// fails, the shipment is rolled back too.
func WithShipmentOutbox(b ShipmentOutboxBuilder) ShipmentRepoOption {
	return func(c *shipmentRepoConfig) { c.shippedOutbox = b }
}

func newShipmentRepoConfig(opts []ShipmentRepoOption) shipmentRepoConfig {
	var c shipmentRepoConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

type ShipmentRepository interface {
	CreateShipment(ctx context.Context, s Shipment) (Shipment, error)
	GetShipment(ctx context.Context, id string) (Shipment, error)
	GetShipmentByTrackingCode(ctx context.Context, trackingCode string) (Shipment, error)
	GetShipmentByOrderID(ctx context.Context, orderID string) (Shipment, error)
	AddShipmentCheckpoint(ctx context.Context, cp ShipmentCheckpoint) (ShipmentCheckpoint, error)
	UpdateShipmentStatus(ctx context.Context, id string, status ShipmentStatus) (Shipment, error)
}

type PostgresShipmentRepository struct {
	pool          *pgxpool.Pool
	outbox        *outbox.Store
	shippedOutbox ShipmentOutboxBuilder
}

func NewPostgresShipmentRepository(pool *pgxpool.Pool, ob *outbox.Store, opts ...ShipmentRepoOption) *PostgresShipmentRepository {
	cfg := newShipmentRepoConfig(opts)
	return &PostgresShipmentRepository{pool: pool, outbox: ob, shippedOutbox: cfg.shippedOutbox}
}

const (
	shipmentColumns   = `id, order_id, carrier, tracking_code, status, created_at, updated_at`
	checkpointColumns = `id, shipment_id, timestamp, location, description, created_at`
)

func scanShipment(row pgx.Row, s *Shipment) error {
	var statusInt int32
	if err := row.Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingCode, &statusInt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return err
	}
	s.Status = ShipmentStatus(statusInt)
	return nil
}

func (r *PostgresShipmentRepository) loadCheckpoints(ctx context.Context, shipmentID string) ([]ShipmentCheckpoint, error) {
	const q = `SELECT ` + checkpointColumns + ` FROM shipment_checkpoints WHERE shipment_id = $1 ORDER BY timestamp ASC, created_at ASC`
	rows, err := r.pool.Query(ctx, q, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []ShipmentCheckpoint
	for rows.Next() {
		var cp ShipmentCheckpoint
		if err := rows.Scan(&cp.ID, &cp.ShipmentID, &cp.Timestamp, &cp.Location, &cp.Description, &cp.CreatedAt); err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, rows.Err()
}

func (r *PostgresShipmentRepository) CreateShipment(ctx context.Context, s Shipment) (Shipment, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = ShipmentStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Shipment{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qShipment = `INSERT INTO shipments (` + shipmentColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := tx.Exec(ctx, qShipment, s.ID, s.OrderID, s.Carrier, s.TrackingCode, int32(s.Status), s.CreatedAt, s.UpdatedAt); err != nil {
		return Shipment{}, fmt.Errorf("insert shipment: %w", err)
	}

	for i := range s.Checkpoints {
		cp := &s.Checkpoints[i]
		if cp.ID == "" {
			cp.ID = uuid.NewString()
		}
		cp.ShipmentID = s.ID
		if cp.Timestamp.IsZero() {
			cp.Timestamp = time.Now()
		}
		cp.CreatedAt = time.Now()

		const qCp = `INSERT INTO shipment_checkpoints (` + checkpointColumns + `)
			VALUES ($1, $2, $3, $4, $5, $6)`
		if _, err := tx.Exec(ctx, qCp, cp.ID, cp.ShipmentID, cp.Timestamp, cp.Location, cp.Description, cp.CreatedAt); err != nil {
			return Shipment{}, fmt.Errorf("insert checkpoint: %w", err)
		}
	}

	// The OrderShipped outbox row commits with the shipment (ADR-0013); a build or
	// enqueue failure rolls the shipment back too.
	if r.shippedOutbox != nil {
		row, err := r.shippedOutbox(s)
		if err != nil {
			return Shipment{}, fmt.Errorf("build order shipped outbox row: %w", err)
		}
		if err := r.outbox.EnqueueTx(ctx, tx, row); err != nil {
			return Shipment{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Shipment{}, fmt.Errorf("commit tx: %w", err)
	}
	return s, nil
}

func (r *PostgresShipmentRepository) GetShipment(ctx context.Context, id string) (Shipment, error) {
	const q = `SELECT ` + shipmentColumns + ` FROM shipments WHERE id = $1`
	var s Shipment
	if err := scanShipment(r.pool.QueryRow(ctx, q, id), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Shipment{}, ErrShipmentNotFound
		}
		return Shipment{}, fmt.Errorf("get shipment %q: %w", id, err)
	}
	cps, err := r.loadCheckpoints(ctx, s.ID)
	if err != nil {
		return Shipment{}, fmt.Errorf("load checkpoints for shipment %q: %w", id, err)
	}
	s.Checkpoints = cps
	return s, nil
}

func (r *PostgresShipmentRepository) GetShipmentByTrackingCode(ctx context.Context, trackingCode string) (Shipment, error) {
	const q = `SELECT ` + shipmentColumns + ` FROM shipments WHERE tracking_code = $1`
	var s Shipment
	if err := scanShipment(r.pool.QueryRow(ctx, q, trackingCode), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Shipment{}, ErrShipmentNotFound
		}
		return Shipment{}, fmt.Errorf("get shipment by tracking code %q: %w", trackingCode, err)
	}
	cps, err := r.loadCheckpoints(ctx, s.ID)
	if err != nil {
		return Shipment{}, fmt.Errorf("load checkpoints for shipment %q: %w", s.ID, err)
	}
	s.Checkpoints = cps
	return s, nil
}

func (r *PostgresShipmentRepository) GetShipmentByOrderID(ctx context.Context, orderID string) (Shipment, error) {
	const q = `SELECT ` + shipmentColumns + ` FROM shipments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`
	var s Shipment
	if err := scanShipment(r.pool.QueryRow(ctx, q, orderID), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Shipment{}, ErrShipmentNotFound
		}
		return Shipment{}, fmt.Errorf("get shipment by order %q: %w", orderID, err)
	}
	cps, err := r.loadCheckpoints(ctx, s.ID)
	if err != nil {
		return Shipment{}, fmt.Errorf("load checkpoints for shipment %q: %w", s.ID, err)
	}
	s.Checkpoints = cps
	return s, nil
}

func (r *PostgresShipmentRepository) AddShipmentCheckpoint(ctx context.Context, cp ShipmentCheckpoint) (ShipmentCheckpoint, error) {
	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}
	cp.CreatedAt = time.Now()

	const q = `INSERT INTO shipment_checkpoints (` + checkpointColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := r.pool.Exec(ctx, q, cp.ID, cp.ShipmentID, cp.Timestamp, cp.Location, cp.Description, cp.CreatedAt); err != nil {
		return ShipmentCheckpoint{}, fmt.Errorf("add shipment checkpoint: %w", err)
	}
	return cp, nil
}

func (r *PostgresShipmentRepository) UpdateShipmentStatus(ctx context.Context, id string, status ShipmentStatus) (Shipment, error) {
	const q = `UPDATE shipments SET status = $2, updated_at = now() WHERE id = $1 RETURNING ` + shipmentColumns
	var s Shipment
	if err := scanShipment(r.pool.QueryRow(ctx, q, id, int32(status)), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Shipment{}, ErrShipmentNotFound
		}
		return Shipment{}, fmt.Errorf("update shipment status %q: %w", id, err)
	}
	cps, err := r.loadCheckpoints(ctx, s.ID)
	if err != nil {
		return Shipment{}, err
	}
	s.Checkpoints = cps
	return s, nil
}
