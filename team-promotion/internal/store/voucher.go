package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

// ErrVoucherNotFound is returned when a voucher lookup misses.
var ErrVoucherNotFound = errors.New("voucher not found")

// Voucher is the persistence-layer view of a voucher. Field semantics mirror
// platform.promotion.v1.Voucher; the mapping to/from the wire type lives in the
// service layer.
type Voucher struct {
	ID            string
	Code          string
	Scope         int32
	SellerID      string
	DiscountType  int32
	DiscountValue int64
	MinSpend      int64
	MaxDiscount   int64
	Quota         int64
	Used          int64
	StartsAt      time.Time
	EndsAt        time.Time
}

// VoucherRepository is the persistence seam for vouchers.
type VoucherRepository interface {
	// Create inserts a new voucher and its outbox row in one transaction. A blank ID
	// is filled with a generated one before event is called with the stored voucher.
	Create(ctx context.Context, v Voucher, event func(Voucher) (outbox.Row, error)) (Voucher, error)
	// GetByCode looks a voucher up by its unique code (ErrVoucherNotFound on miss).
	GetByCode(ctx context.Context, code string) (Voucher, error)
	// GetByID looks a voucher up by surrogate id (ErrVoucherNotFound on miss).
	GetByID(ctx context.Context, id string) (Voucher, error)
	// List returns vouchers for a seller (empty sellerID = platform-scoped),
	// newest first, bounded by limit/offset.
	List(ctx context.Context, sellerID string, limit, offset int) ([]Voucher, error)
	// IncrementUsed atomically adds delta to a voucher's used counter. Called on
	// commit to finalize a redemption.
	IncrementUsed(ctx context.Context, id string, delta int64) error
	// Ping verifies the backing store is reachable.
	Ping(ctx context.Context) error
}

// ── Postgres implementation ──

// PostgresVoucherRepository persists vouchers in promotion_db.
type PostgresVoucherRepository struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPostgresVoucherRepository(pool *pgxpool.Pool, ob *outbox.Store) *PostgresVoucherRepository {
	return &PostgresVoucherRepository{pool: pool, outbox: ob}
}

const voucherColumns = `id, code, scope, seller_id, discount_type, discount_value, min_spend, max_discount, quota, used, starts_at, ends_at`

func scanVoucher(row pgx.Row, v *Voucher) error {
	return row.Scan(
		&v.ID, &v.Code, &v.Scope, &v.SellerID, &v.DiscountType, &v.DiscountValue,
		&v.MinSpend, &v.MaxDiscount, &v.Quota, &v.Used, &v.StartsAt, &v.EndsAt,
	)
}

func (r *PostgresVoucherRepository) Create(ctx context.Context, v Voucher, event func(Voucher) (outbox.Row, error)) (Voucher, error) {
	if v.ID == "" {
		v.ID = NewID()
	}
	const q = `INSERT INTO vouchers (` + voucherColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, q,
			v.ID, v.Code, v.Scope, v.SellerID, v.DiscountType, v.DiscountValue,
			v.MinSpend, v.MaxDiscount, v.Quota, v.Used, nullableTime(v.StartsAt), nullableTime(v.EndsAt),
		); err != nil {
			return fmt.Errorf("insert voucher: %w", err)
		}
		row, err := event(v)
		if err != nil {
			return err
		}
		return r.outbox.EnqueueTx(ctx, tx, row)
	})
	if err != nil {
		return Voucher{}, err
	}
	return v, nil
}

func (r *PostgresVoucherRepository) GetByCode(ctx context.Context, code string) (Voucher, error) {
	const q = `SELECT ` + voucherColumns + ` FROM vouchers WHERE code = $1`
	var v Voucher
	if err := scanVoucher(r.pool.QueryRow(ctx, q, code), &v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Voucher{}, ErrVoucherNotFound
		}
		return Voucher{}, fmt.Errorf("get voucher by code %q: %w", code, err)
	}
	return v, nil
}

func (r *PostgresVoucherRepository) GetByID(ctx context.Context, id string) (Voucher, error) {
	const q = `SELECT ` + voucherColumns + ` FROM vouchers WHERE id = $1`
	var v Voucher
	if err := scanVoucher(r.pool.QueryRow(ctx, q, id), &v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Voucher{}, ErrVoucherNotFound
		}
		return Voucher{}, fmt.Errorf("get voucher by id %q: %w", id, err)
	}
	return v, nil
}

func (r *PostgresVoucherRepository) List(ctx context.Context, sellerID string, limit, offset int) ([]Voucher, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT ` + voucherColumns + ` FROM vouchers WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, sellerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list vouchers: %w", err)
	}
	defer rows.Close()
	var out []Voucher
	for rows.Next() {
		var v Voucher
		if err := scanVoucher(rows, &v); err != nil {
			return nil, fmt.Errorf("scan voucher: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *PostgresVoucherRepository) IncrementUsed(ctx context.Context, id string, delta int64) error {
	const q = `UPDATE vouchers SET used = used + $2, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, delta)
	if err != nil {
		return fmt.Errorf("increment voucher used: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrVoucherNotFound
	}
	return nil
}

func (r *PostgresVoucherRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// nullableTime maps a zero time.Time to nil so it is stored as SQL NULL.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
