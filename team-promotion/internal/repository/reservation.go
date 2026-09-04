package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrReservationNotFound is returned when a reservation lookup misses.
var ErrReservationNotFound = errors.New("voucher reservation not found")

// Reservation status values. A hold is created RESERVED, then finalized to
// COMMITTED (redemption counted) or freed to RELEASED.
const (
	ReservationReserved  = "reserved"
	ReservationCommitted = "committed"
	ReservationReleased  = "released"
)

// Reservation is a voucher hold created by ValidateAndReserve and closed by
// Commit/Release. It is idempotent on ReservationID (the order saga id), mirroring
// team-domain's stock reservation (ADR-0008).
type Reservation struct {
	ID             string
	ReservationID  string
	VoucherID      string
	BuyerID        string
	DiscountAmount int64
	Status         string
}

// ReservationRepository is the persistence seam for the voucher redemption hold.
type ReservationRepository interface {
	// Create inserts a hold. It is idempotent on ReservationID: if a row for that
	// reservation_id already exists the existing row is returned with created=false
	// and no second hold is made (so quota is never decremented twice).
	Create(ctx context.Context, r Reservation) (stored Reservation, created bool, err error)
	// GetByReservationID fetches a hold by the caller's reservation_id.
	GetByReservationID(ctx context.Context, reservationID string) (Reservation, error)
	// UpdateStatus transitions a hold; returns ErrReservationNotFound if absent.
	UpdateStatus(ctx context.Context, reservationID, status string) error
	// Ping verifies the backing store is reachable.
	Ping(ctx context.Context) error
}

// ── Postgres implementation ──

// PostgresReservationRepository persists reservations in promotion_db.
type PostgresReservationRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresReservationRepository(pool *pgxpool.Pool) *PostgresReservationRepository {
	return &PostgresReservationRepository{pool: pool}
}

const reservationColumns = `id, reservation_id, voucher_id, buyer_id, discount_amount, status`

func scanReservation(row pgx.Row, r *Reservation) error {
	return row.Scan(&r.ID, &r.ReservationID, &r.VoucherID, &r.BuyerID, &r.DiscountAmount, &r.Status)
}

func (r *PostgresReservationRepository) Create(ctx context.Context, res Reservation) (Reservation, bool, error) {
	if res.ID == "" {
		res.ID = newID()
	}
	if res.Status == "" {
		res.Status = ReservationReserved
	}
	// UNIQUE(reservation_id) enforces idempotency; ON CONFLICT DO NOTHING makes a
	// retried reserve a no-op. We then read the surviving row back so the caller
	// always sees the authoritative hold (its original discount).
	const ins = `INSERT INTO voucher_reservations (` + reservationColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (reservation_id) DO NOTHING`
	ct, err := r.pool.Exec(ctx, ins, res.ID, res.ReservationID, res.VoucherID, res.BuyerID, res.DiscountAmount, res.Status)
	if err != nil {
		return Reservation{}, false, fmt.Errorf("insert reservation: %w", err)
	}
	if ct.RowsAffected() == 1 {
		return res, true, nil
	}
	existing, err := r.GetByReservationID(ctx, res.ReservationID)
	if err != nil {
		return Reservation{}, false, err
	}
	return existing, false, nil
}

func (r *PostgresReservationRepository) GetByReservationID(ctx context.Context, reservationID string) (Reservation, error) {
	const q = `SELECT ` + reservationColumns + ` FROM voucher_reservations WHERE reservation_id = $1`
	var res Reservation
	if err := scanReservation(r.pool.QueryRow(ctx, q, reservationID), &res); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Reservation{}, ErrReservationNotFound
		}
		return Reservation{}, fmt.Errorf("get reservation %q: %w", reservationID, err)
	}
	return res, nil
}

func (r *PostgresReservationRepository) UpdateStatus(ctx context.Context, reservationID, status string) error {
	const q = `UPDATE voucher_reservations SET status = $2, updated_at = now() WHERE reservation_id = $1`
	ct, err := r.pool.Exec(ctx, q, reservationID, status)
	if err != nil {
		return fmt.Errorf("update reservation status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrReservationNotFound
	}
	return nil
}

func (r *PostgresReservationRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// ── In-memory implementation ──

// InMemoryReservationRepository is the DB-less backend used when DATABASE_ENABLED
// is false and in unit tests.
type InMemoryReservationRepository struct {
	mu           sync.RWMutex
	reservations map[string]Reservation // keyed by reservation_id
}

func NewInMemoryReservationRepository() *InMemoryReservationRepository {
	return &InMemoryReservationRepository{reservations: make(map[string]Reservation)}
}

func (r *InMemoryReservationRepository) Create(_ context.Context, res Reservation) (Reservation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.reservations[res.ReservationID]; ok {
		return existing, false, nil
	}
	if res.ID == "" {
		res.ID = newID()
	}
	if res.Status == "" {
		res.Status = ReservationReserved
	}
	r.reservations[res.ReservationID] = res
	return res, true, nil
}

func (r *InMemoryReservationRepository) GetByReservationID(_ context.Context, reservationID string) (Reservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return Reservation{}, ErrReservationNotFound
	}
	return res, nil
}

func (r *InMemoryReservationRepository) UpdateStatus(_ context.Context, reservationID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return ErrReservationNotFound
	}
	res.Status = status
	r.reservations[reservationID] = res
	return nil
}

func (r *InMemoryReservationRepository) Ping(_ context.Context) error { return nil }
