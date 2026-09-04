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

// ErrReservationNotFound is returned when a reservation row is absent.
var ErrReservationNotFound = errors.New("reservation not found")

// SagaStatus tracks the lifecycle of a checkout saga.
type SagaStatus int32

const (
	SagaStatusUnspecified SagaStatus = 0
	SagaStatusPending     SagaStatus = 1
	SagaStatusCompleted   SagaStatus = 2
	SagaStatusCompensated SagaStatus = 3
	SagaStatusFailed      SagaStatus = 4
)

// ReservationStatus tracks a single stock reservation within a saga.
type ReservationStatus int32

const (
	ReservationStatusUnspecified ReservationStatus = 0
	// ReservationStatusPending: intent persisted, ReserveStock not yet confirmed.
	ReservationStatusPending ReservationStatus = 1
	// ReservationStatusReserved: stock decremented in team-domain (holds stock).
	ReservationStatusReserved ReservationStatus = 2
	// ReservationStatusCommitted: owning seller-order persisted; never released (M7).
	ReservationStatusCommitted ReservationStatus = 3
	// ReservationStatusReleased: stock returned via compensation.
	ReservationStatusReleased ReservationStatus = 4
	// ReservationStatusReleaseFailed: release attempt failed; parked for the sweep.
	ReservationStatusReleaseFailed ReservationStatus = 5
	// ReservationStatusFailed: ReserveStock itself was rejected (no stock held).
	ReservationStatusFailed ReservationStatus = 6
)

// Saga is the durable header row for one checkout attempt.
type Saga struct {
	ID        string
	BuyerID   string
	Status    SagaStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Reservation is one durable stock-reservation intent. It is persisted BEFORE the
// external ReserveStock call (AD3) and keyed by a stable reservation_id per
// (cart_item, attempt) (AD5/M6).
type Reservation struct {
	ID        string
	SagaID    string
	OrderID   string
	SellerID  string
	BuyerID   string
	ListingID string
	VariantID string
	Quantity  int32
	Status    ReservationStatus
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SagaRepository persists saga + reservation state so the purchase path is
// recoverable across restarts and compensations are never lost.
type SagaRepository interface {
	CreateSaga(ctx context.Context, s Saga) (Saga, error)
	UpdateSagaStatus(ctx context.Context, id string, status SagaStatus) error

	CreateReservation(ctx context.Context, r Reservation) (Reservation, error)
	GetReservation(ctx context.Context, id string) (Reservation, error)
	UpdateReservationStatus(ctx context.Context, id string, status ReservationStatus) error
	// CommitReservation marks a reservation COMMITTED and records its owning order.
	// A committed reservation is never released by compensation or the sweep (M7).
	CommitReservation(ctx context.Context, id, orderID string) error
	ListReservationsBySaga(ctx context.Context, sagaID string) ([]Reservation, error)
	// FindReleasable returns reservations that still hold stock (RESERVED or
	// RELEASE_FAILED, never COMMITTED) whose TTL has elapsed — the sweep set.
	FindReleasable(ctx context.Context, now time.Time, limit int) ([]Reservation, error)
}

// ── Postgres implementation ──

type PostgresSagaRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSagaRepository(pool *pgxpool.Pool) *PostgresSagaRepository {
	return &PostgresSagaRepository{pool: pool}
}

const reservationColumns = `id, saga_id, order_id, seller_id, buyer_id, listing_id, variant_id, quantity, status, expires_at, created_at, updated_at`

func scanReservation(row pgx.Row, r *Reservation) error {
	var statusInt int32
	var orderID sql.NullString
	if err := row.Scan(&r.ID, &r.SagaID, &orderID, &r.SellerID, &r.BuyerID, &r.ListingID, &r.VariantID, &r.Quantity, &statusInt, &r.ExpiresAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return err
	}
	r.OrderID = orderID.String
	r.Status = ReservationStatus(statusInt)
	return nil
}

func (r *PostgresSagaRepository) CreateSaga(ctx context.Context, s Saga) (Saga, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = SagaStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	const q = `INSERT INTO order_sagas (id, buyer_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := r.pool.Exec(ctx, q, s.ID, s.BuyerID, int32(s.Status), s.CreatedAt, s.UpdatedAt); err != nil {
		return Saga{}, fmt.Errorf("insert saga: %w", err)
	}
	return s, nil
}

func (r *PostgresSagaRepository) UpdateSagaStatus(ctx context.Context, id string, status SagaStatus) error {
	const q = `UPDATE order_sagas SET status = $2, updated_at = now() WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, int32(status)); err != nil {
		return fmt.Errorf("update saga status: %w", err)
	}
	return nil
}

func (r *PostgresSagaRepository) CreateReservation(ctx context.Context, res Reservation) (Reservation, error) {
	if res.ID == "" {
		res.ID = uuid.NewString()
	}
	if res.Status == 0 {
		res.Status = ReservationStatusPending
	}
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(15 * time.Minute)
	}
	res.CreatedAt = time.Now()
	res.UpdatedAt = time.Now()
	// Idempotent on the stable reservation_id (AD5): a retried checkout re-inserts
	// the same row as a no-op rather than creating a duplicate reservation.
	const q = `INSERT INTO order_reservations (` + reservationColumns + `)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, res.ID, res.SagaID, res.OrderID, res.SellerID, res.BuyerID, res.ListingID, res.VariantID, res.Quantity, int32(res.Status), res.ExpiresAt, res.CreatedAt, res.UpdatedAt); err != nil {
		return Reservation{}, fmt.Errorf("insert reservation: %w", err)
	}
	return res, nil
}

func (r *PostgresSagaRepository) GetReservation(ctx context.Context, id string) (Reservation, error) {
	const q = `SELECT ` + reservationColumns + ` FROM order_reservations WHERE id = $1`
	var res Reservation
	if err := scanReservation(r.pool.QueryRow(ctx, q, id), &res); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Reservation{}, ErrReservationNotFound
		}
		return Reservation{}, fmt.Errorf("get reservation %q: %w", id, err)
	}
	return res, nil
}

func (r *PostgresSagaRepository) UpdateReservationStatus(ctx context.Context, id string, status ReservationStatus) error {
	const q = `UPDATE order_reservations SET status = $2, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, int32(status))
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrReservationNotFound
	}
	return nil
}

func (r *PostgresSagaRepository) CommitReservation(ctx context.Context, id, orderID string) error {
	const q = `UPDATE order_reservations SET status = $2, order_id = $3, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, int32(ReservationStatusCommitted), orderID)
	if err != nil {
		return fmt.Errorf("commit reservation: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrReservationNotFound
	}
	return nil
}

func (r *PostgresSagaRepository) ListReservationsBySaga(ctx context.Context, sagaID string) ([]Reservation, error) {
	const q = `SELECT ` + reservationColumns + ` FROM order_reservations WHERE saga_id = $1 ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, q, sagaID)
	if err != nil {
		return nil, fmt.Errorf("list reservations by saga: %w", err)
	}
	defer rows.Close()
	var out []Reservation
	for rows.Next() {
		var res Reservation
		if err := scanReservation(rows, &res); err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

func (r *PostgresSagaRepository) FindReleasable(ctx context.Context, now time.Time, limit int) ([]Reservation, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT ` + reservationColumns + ` FROM order_reservations
		WHERE status IN ($1, $2) AND expires_at <= $3
		ORDER BY expires_at ASC LIMIT $4`
	rows, err := r.pool.Query(ctx, q, int32(ReservationStatusReserved), int32(ReservationStatusReleaseFailed), now, limit)
	if err != nil {
		return nil, fmt.Errorf("find releasable reservations: %w", err)
	}
	defer rows.Close()
	var out []Reservation
	for rows.Next() {
		var res Reservation
		if err := scanReservation(rows, &res); err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// ── In-memory implementation ──

// reservationStore is the backing state shared by InMemorySagaRepository
// instances. Sharing one store across two repositories models a process restart
// against the same durable database (used by the M8 restart test).
type reservationStore struct {
	mu           sync.RWMutex
	sagas        map[string]Saga
	reservations map[string]Reservation
}

// NewReservationStore builds an empty shared backing store.
func NewReservationStore() *reservationStore {
	return &reservationStore{
		sagas:        make(map[string]Saga),
		reservations: make(map[string]Reservation),
	}
}

type InMemorySagaRepository struct {
	store *reservationStore
}

// NewInMemorySagaRepository builds an in-memory saga repo over its own store.
func NewInMemorySagaRepository() *InMemorySagaRepository {
	return &InMemorySagaRepository{store: NewReservationStore()}
}

// NewInMemorySagaRepositoryWithStore builds a repo over an externally-owned store
// so a second instance can read state a prior instance wrote (restart semantics).
func NewInMemorySagaRepositoryWithStore(store *reservationStore) *InMemorySagaRepository {
	if store == nil {
		store = NewReservationStore()
	}
	return &InMemorySagaRepository{store: store}
}

func (r *InMemorySagaRepository) CreateSaga(_ context.Context, s Saga) (Saga, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = SagaStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	r.store.sagas[s.ID] = s
	return s, nil
}

func (r *InMemorySagaRepository) UpdateSagaStatus(_ context.Context, id string, status SagaStatus) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	s, ok := r.store.sagas[id]
	if !ok {
		return fmt.Errorf("saga %q not found", id)
	}
	s.Status = status
	s.UpdatedAt = time.Now()
	r.store.sagas[id] = s
	return nil
}

func (r *InMemorySagaRepository) CreateReservation(_ context.Context, res Reservation) (Reservation, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if res.ID == "" {
		res.ID = uuid.NewString()
	}
	// Idempotent on the stable reservation_id (AD5): keep the existing row.
	if existing, ok := r.store.reservations[res.ID]; ok {
		return existing, nil
	}
	if res.Status == 0 {
		res.Status = ReservationStatusPending
	}
	if res.ExpiresAt.IsZero() {
		res.ExpiresAt = time.Now().Add(15 * time.Minute)
	}
	res.CreatedAt = time.Now()
	res.UpdatedAt = time.Now()
	r.store.reservations[res.ID] = res
	return res, nil
}

func (r *InMemorySagaRepository) GetReservation(_ context.Context, id string) (Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return Reservation{}, ErrReservationNotFound
	}
	return res, nil
}

func (r *InMemorySagaRepository) UpdateReservationStatus(_ context.Context, id string, status ReservationStatus) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return ErrReservationNotFound
	}
	res.Status = status
	res.UpdatedAt = time.Now()
	r.store.reservations[id] = res
	return nil
}

func (r *InMemorySagaRepository) CommitReservation(_ context.Context, id, orderID string) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	res, ok := r.store.reservations[id]
	if !ok {
		return ErrReservationNotFound
	}
	res.Status = ReservationStatusCommitted
	res.OrderID = orderID
	res.UpdatedAt = time.Now()
	r.store.reservations[id] = res
	return nil
}

func (r *InMemorySagaRepository) ListReservationsBySaga(_ context.Context, sagaID string) ([]Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	var out []Reservation
	for _, res := range r.store.reservations {
		if res.SagaID == sagaID {
			out = append(out, res)
		}
	}
	return out, nil
}

func (r *InMemorySagaRepository) FindReleasable(_ context.Context, now time.Time, limit int) ([]Reservation, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []Reservation
	for _, res := range r.store.reservations {
		if res.Status != ReservationStatusReserved && res.Status != ReservationStatusReleaseFailed {
			continue
		}
		if res.ExpiresAt.After(now) {
			continue
		}
		out = append(out, res)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
