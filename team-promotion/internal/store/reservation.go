package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

var ErrPerUserLimit = errors.New("per-user voucher limit reached")

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
	ExpiresAt      time.Time
}

// ReservationRepository is the persistence seam for the voucher redemption hold.
type ReservationRepository interface {
	// Create inserts a hold. It is idempotent on ReservationID: if a row for that
	// reservation_id already exists the existing row is returned with created=false
	// and no second hold is made (so quota is never decremented twice).
	Create(ctx context.Context, r Reservation) (stored Reservation, created bool, err error)
	CreateWithinLimit(ctx context.Context, r Reservation, perUserLimit int, now time.Time) (stored Reservation, created bool, err error)
	CountActiveByBuyer(ctx context.Context, voucherID, buyerID string, now time.Time) (int, error)
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

const reservationColumns = `id, reservation_id, voucher_id, buyer_id, discount_amount, status, expires_at`

func scanReservation(row pgx.Row, r *Reservation) error {
	return row.Scan(&r.ID, &r.ReservationID, &r.VoucherID, &r.BuyerID, &r.DiscountAmount, &r.Status, &r.ExpiresAt)
}

func (r *PostgresReservationRepository) Create(ctx context.Context, res Reservation) (Reservation, bool, error) {
	if res.ID == "" {
		res.ID = NewID()
	}
	if res.Status == "" {
		res.Status = ReservationReserved
	}
	res.ExpiresAt = expiryOrDefault(res.ExpiresAt)
	// UNIQUE(reservation_id) enforces idempotency; ON CONFLICT DO NOTHING makes a
	// retried reserve a no-op. We then read the surviving row back so the caller
	// always sees the authoritative hold (its original discount).
	const ins = `INSERT INTO voucher_reservations (` + reservationColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (reservation_id) DO NOTHING`
	ct, err := r.pool.Exec(ctx, ins, res.ID, res.ReservationID, res.VoucherID, res.BuyerID, res.DiscountAmount, res.Status, res.ExpiresAt)
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

const defaultHoldTTL = 15 * time.Minute

func expiryOrDefault(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().Add(defaultHoldTTL)
	}
	return t
}

func (r *PostgresReservationRepository) CountActiveByBuyer(ctx context.Context, voucherID, buyerID string, now time.Time) (int, error) {
	n, err := countActive(ctx, r.pool, voucherID, buyerID, now)
	if err != nil {
		return 0, fmt.Errorf("count active holds: %w", err)
	}
	return n, nil
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func countActive(ctx context.Context, q queryRower, voucherID, buyerID string, now time.Time) (int, error) {
	const cq = `SELECT count(*) FROM voucher_reservations
		WHERE voucher_id = $1 AND buyer_id = $2
			AND (status = 'committed' OR (status = 'reserved' AND expires_at > $3))`
	var n int
	err := q.QueryRow(ctx, cq, voucherID, buyerID, now).Scan(&n)
	return n, err
}

func (r *PostgresReservationRepository) CreateWithinLimit(ctx context.Context, res Reservation, perUserLimit int, now time.Time) (Reservation, bool, error) {
	if res.ID == "" {
		res.ID = NewID()
	}
	if res.Status == "" {
		res.Status = ReservationReserved
	}
	res.ExpiresAt = expiryOrDefault(res.ExpiresAt)
	stored, created := res, false
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, res.VoucherID+"|"+res.BuyerID); err != nil {
			return fmt.Errorf("lock buyer holds: %w", err)
		}
		var existing Reservation
		err := scanReservation(tx.QueryRow(ctx, `SELECT `+reservationColumns+` FROM voucher_reservations WHERE reservation_id = $1`, res.ReservationID), &existing)
		if err == nil {
			stored = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("get reservation %q: %w", res.ReservationID, err)
		}
		n, err := countActive(ctx, tx, res.VoucherID, res.BuyerID, now)
		if err != nil {
			return fmt.Errorf("count active holds: %w", err)
		}
		if n >= perUserLimit {
			return ErrPerUserLimit
		}
		if _, err := tx.Exec(ctx, `INSERT INTO voucher_reservations (`+reservationColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			res.ID, res.ReservationID, res.VoucherID, res.BuyerID, res.DiscountAmount, res.Status, res.ExpiresAt); err != nil {
			return fmt.Errorf("insert reservation: %w", err)
		}
		created = true
		return nil
	})
	if err != nil {
		return Reservation{}, false, err
	}
	return stored, created, nil
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
