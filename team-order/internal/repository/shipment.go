package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrShipmentNotFound = errors.New("shipment not found")
)

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
	pool *pgxpool.Pool
}

func NewPostgresShipmentRepository(pool *pgxpool.Pool) *PostgresShipmentRepository {
	return &PostgresShipmentRepository{pool: pool}
}

const shipmentColumns = `id, order_id, carrier, tracking_code, status, created_at, updated_at`
const checkpointColumns = `id, shipment_id, timestamp, location, description, created_at`

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
	defer tx.Rollback(ctx)

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

// InMemoryShipmentRepository
type InMemoryShipmentRepository struct {
	mu          sync.RWMutex
	shipments   map[string]Shipment
	checkpoints map[string][]ShipmentCheckpoint
}

func NewInMemoryShipmentRepository() *InMemoryShipmentRepository {
	return &InMemoryShipmentRepository{
		shipments:   make(map[string]Shipment),
		checkpoints: make(map[string][]ShipmentCheckpoint),
	}
}

func (r *InMemoryShipmentRepository) CreateShipment(_ context.Context, s Shipment) (Shipment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = ShipmentStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()

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
	}

	r.shipments[s.ID] = s
	r.checkpoints[s.ID] = append([]ShipmentCheckpoint{}, s.Checkpoints...)
	return s, nil
}

func (r *InMemoryShipmentRepository) GetShipment(_ context.Context, id string) (Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.shipments[id]
	if !ok {
		return Shipment{}, ErrShipmentNotFound
	}
	s.Checkpoints = append([]ShipmentCheckpoint{}, r.checkpoints[id]...)
	return s, nil
}

func (r *InMemoryShipmentRepository) GetShipmentByTrackingCode(_ context.Context, trackingCode string) (Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.shipments {
		if s.TrackingCode == trackingCode {
			s.Checkpoints = append([]ShipmentCheckpoint{}, r.checkpoints[s.ID]...)
			return s, nil
		}
	}
	return Shipment{}, ErrShipmentNotFound
}

func (r *InMemoryShipmentRepository) GetShipmentByOrderID(_ context.Context, orderID string) (Shipment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.shipments {
		if s.OrderID == orderID {
			s.Checkpoints = append([]ShipmentCheckpoint{}, r.checkpoints[s.ID]...)
			return s, nil
		}
	}
	return Shipment{}, ErrShipmentNotFound
}

func (r *InMemoryShipmentRepository) AddShipmentCheckpoint(_ context.Context, cp ShipmentCheckpoint) (ShipmentCheckpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.shipments[cp.ShipmentID]; !ok {
		return ShipmentCheckpoint{}, ErrShipmentNotFound
	}

	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}
	cp.CreatedAt = time.Now()

	r.checkpoints[cp.ShipmentID] = append(r.checkpoints[cp.ShipmentID], cp)
	return cp, nil
}

func (r *InMemoryShipmentRepository) UpdateShipmentStatus(_ context.Context, id string, status ShipmentStatus) (Shipment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.shipments[id]
	if !ok {
		return Shipment{}, ErrShipmentNotFound
	}
	s.Status = status
	s.UpdatedAt = time.Now()
	r.shipments[id] = s
	s.Checkpoints = append([]ShipmentCheckpoint{}, r.checkpoints[id]...)
	return s, nil
}
