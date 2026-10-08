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

// ErrSagaNotFound is returned when a saga row is absent.
var ErrSagaNotFound = errors.New("saga not found")

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
	ID      string
	BuyerID string
	// IdempotencyKey is the client's Idempotency-Key ("" when none). Unique per
	// buyer while set; compensation clears it so the key can be reused.
	IdempotencyKey string
	Status         SagaStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	// CreateSaga inserts a saga. With an IdempotencyKey it is insert-or-lookup on
	// (buyer, key): created=false returns the saga that already holds the key.
	CreateSaga(ctx context.Context, s Saga) (saga Saga, created bool, err error)
	GetSaga(ctx context.Context, id string) (Saga, error)
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
	// FindHeldByCancelledOrders returns reservations still holding stock (COMMITTED
	// or RELEASE_FAILED) whose order is Cancelled and was last updated at or before
	// cancelledBefore — a crash between a cancel's claim and its release.
	FindHeldByCancelledOrders(ctx context.Context, cancelledBefore time.Time, limit int) ([]Reservation, error)
	// FindStalePendingSagas returns sagas still PENDING that were created at or
	// before createdBefore (a checkout attempt that crashed or never finished).
	FindStalePendingSagas(ctx context.Context, createdBefore time.Time, limit int) ([]Saga, error)
}

// OrderReader is the slice of an order store the in-memory saga repository needs
// to answer FindHeldByCancelledOrders (Postgres joins the orders table instead).
type OrderReader interface {
	GetOrder(ctx context.Context, id string) (Order, error)
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

func (r *PostgresSagaRepository) CreateSaga(ctx context.Context, s Saga) (Saga, bool, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = SagaStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	if s.IdempotencyKey == "" {
		const q = `INSERT INTO order_sagas (id, buyer_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)`
		if _, err := r.pool.Exec(ctx, q, s.ID, s.BuyerID, int32(s.Status), s.CreatedAt, s.UpdatedAt); err != nil {
			return Saga{}, false, fmt.Errorf("insert saga: %w", err)
		}
		return s, true, nil
	}

	// Insert-or-lookup on the partial unique index (buyer_id, idempotency_key).
	const ins = `INSERT INTO order_sagas (id, buyer_id, status, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (buyer_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`
	const sel = `SELECT ` + sagaColumns + ` FROM order_sagas WHERE buyer_id = $1 AND idempotency_key = $2`
	// The holder may free the key (compensation) between our conflicting insert and
	// our lookup; the next insert then succeeds, so retry a few times.
	for attempt := 0; attempt < 3; attempt++ {
		ct, err := r.pool.Exec(ctx, ins, s.ID, s.BuyerID, int32(s.Status), s.IdempotencyKey, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return Saga{}, false, fmt.Errorf("insert saga: %w", err)
		}
		if ct.RowsAffected() == 1 {
			return s, true, nil
		}
		var existing Saga
		if err := scanSaga(r.pool.QueryRow(ctx, sel, s.BuyerID, s.IdempotencyKey), &existing); err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return Saga{}, false, fmt.Errorf("lookup saga by idempotency key: %w", err)
		}
		return existing, false, nil
	}
	return Saga{}, false, fmt.Errorf("idempotency key contended for buyer %q; retry", s.BuyerID)
}

const sagaColumns = `id, buyer_id, COALESCE(idempotency_key, ''), status, created_at, updated_at`

func scanSaga(row pgx.Row, s *Saga) error {
	var statusInt int32
	if err := row.Scan(&s.ID, &s.BuyerID, &s.IdempotencyKey, &statusInt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return err
	}
	s.Status = SagaStatus(statusInt)
	return nil
}

func (r *PostgresSagaRepository) GetSaga(ctx context.Context, id string) (Saga, error) {
	var s Saga
	if err := scanSaga(r.pool.QueryRow(ctx, `SELECT `+sagaColumns+` FROM order_sagas WHERE id = $1`, id), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Saga{}, ErrSagaNotFound
		}
		return Saga{}, fmt.Errorf("get saga %q: %w", id, err)
	}
	return s, nil
}

func (r *PostgresSagaRepository) UpdateSagaStatus(ctx context.Context, id string, status SagaStatus) error {
	// COMPENSATED / FAILED free the idempotency key in the same statement, so the
	// client's next request with that key runs a fresh checkout.
	const q = `UPDATE order_sagas SET status = $2, updated_at = now(),
		idempotency_key = CASE WHEN $2 IN (3, 4) THEN NULL ELSE idempotency_key END
		WHERE id = $1`
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

func scanReservations(rows pgx.Rows) ([]Reservation, error) {
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

func (r *PostgresSagaRepository) FindHeldByCancelledOrders(ctx context.Context, cancelledBefore time.Time, limit int) ([]Reservation, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT r.id, r.saga_id, r.order_id, r.seller_id, r.buyer_id, r.listing_id, r.variant_id, r.quantity, r.status, r.expires_at, r.created_at, r.updated_at
		FROM order_reservations r
		JOIN orders o ON o.id = r.order_id
		WHERE o.status = $1 AND o.updated_at <= $2 AND r.status IN ($3, $4)
		ORDER BY r.updated_at ASC, r.id ASC LIMIT $5`
	rows, err := r.pool.Query(ctx, q, int32(OrderStatusCancelled), cancelledBefore,
		int32(ReservationStatusCommitted), int32(ReservationStatusReleaseFailed), limit)
	if err != nil {
		return nil, fmt.Errorf("find reservations held by cancelled orders: %w", err)
	}
	return scanReservations(rows)
}

func (r *PostgresSagaRepository) FindStalePendingSagas(ctx context.Context, createdBefore time.Time, limit int) ([]Saga, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT ` + sagaColumns + ` FROM order_sagas
		WHERE status = $1 AND created_at <= $2 ORDER BY created_at ASC LIMIT $3`
	rows, err := r.pool.Query(ctx, q, int32(SagaStatusPending), createdBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("find stale pending sagas: %w", err)
	}
	defer rows.Close()
	var out []Saga
	for rows.Next() {
		var sg Saga
		if err := scanSaga(rows, &sg); err != nil {
			return nil, err
		}
		out = append(out, sg)
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
	store  *reservationStore
	orders OrderReader // set by BindOrders; answers FindHeldByCancelledOrders
}

// BindOrders gives the in-memory saga repository the order store it reads order
// statuses from (Postgres joins). NewOrderService binds it automatically.
func (r *InMemorySagaRepository) BindOrders(o OrderReader) { r.orders = o }

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

func (r *InMemorySagaRepository) CreateSaga(_ context.Context, s Saga) (Saga, bool, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if s.IdempotencyKey != "" {
		for _, existing := range r.store.sagas {
			if existing.BuyerID == s.BuyerID && existing.IdempotencyKey == s.IdempotencyKey {
				return existing, false, nil
			}
		}
	}
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Status == 0 {
		s.Status = SagaStatusPending
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	r.store.sagas[s.ID] = s
	return s, true, nil
}

func (r *InMemorySagaRepository) GetSaga(_ context.Context, id string) (Saga, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	s, ok := r.store.sagas[id]
	if !ok {
		return Saga{}, ErrSagaNotFound
	}
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
	if status == SagaStatusCompensated || status == SagaStatusFailed {
		s.IdempotencyKey = "" // free the key (mirrors the Postgres statement)
	}
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

func (r *InMemorySagaRepository) FindHeldByCancelledOrders(ctx context.Context, cancelledBefore time.Time, limit int) ([]Reservation, error) {
	if r.orders == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	r.store.mu.RLock()
	var candidates []Reservation
	for _, res := range r.store.reservations {
		if res.OrderID != "" && (res.Status == ReservationStatusCommitted || res.Status == ReservationStatusReleaseFailed) {
			candidates = append(candidates, res)
		}
	}
	r.store.mu.RUnlock() // never hold the store lock while reading orders

	var out []Reservation
	for _, res := range candidates {
		o, err := r.orders.GetOrder(ctx, res.OrderID)
		if err != nil || o.Status != OrderStatusCancelled || o.UpdatedAt.After(cancelledBefore) {
			continue
		}
		out = append(out, res)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *InMemorySagaRepository) FindStalePendingSagas(_ context.Context, createdBefore time.Time, limit int) ([]Saga, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []Saga
	for _, sg := range r.store.sagas {
		if sg.Status == SagaStatusPending && !sg.CreatedAt.After(createdBefore) {
			out = append(out, sg)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
