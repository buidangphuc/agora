package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	// Create inserts a new voucher. A blank ID is filled with a generated one.
	Create(ctx context.Context, v Voucher) (Voucher, error)
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
	pool *pgxpool.Pool
}

func NewPostgresVoucherRepository(pool *pgxpool.Pool) *PostgresVoucherRepository {
	return &PostgresVoucherRepository{pool: pool}
}

const voucherColumns = `id, code, scope, seller_id, discount_type, discount_value, min_spend, max_discount, quota, used, starts_at, ends_at`

func scanVoucher(row pgx.Row, v *Voucher) error {
	return row.Scan(
		&v.ID, &v.Code, &v.Scope, &v.SellerID, &v.DiscountType, &v.DiscountValue,
		&v.MinSpend, &v.MaxDiscount, &v.Quota, &v.Used, &v.StartsAt, &v.EndsAt,
	)
}

func (r *PostgresVoucherRepository) Create(ctx context.Context, v Voucher) (Voucher, error) {
	if v.ID == "" {
		v.ID = newID()
	}
	const q = `INSERT INTO vouchers (` + voucherColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	if _, err := r.pool.Exec(ctx, q,
		v.ID, v.Code, v.Scope, v.SellerID, v.DiscountType, v.DiscountValue,
		v.MinSpend, v.MaxDiscount, v.Quota, v.Used, nullableTime(v.StartsAt), nullableTime(v.EndsAt),
	); err != nil {
		return Voucher{}, fmt.Errorf("insert voucher: %w", err)
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

// ── In-memory implementation ──

// InMemoryVoucherRepository is the DB-less backend used when DATABASE_ENABLED=false
// and in unit tests.
type InMemoryVoucherRepository struct {
	mu       sync.RWMutex
	vouchers map[string]Voucher // keyed by id
}

func NewInMemoryVoucherRepository() *InMemoryVoucherRepository {
	return &InMemoryVoucherRepository{vouchers: make(map[string]Voucher)}
}

func (r *InMemoryVoucherRepository) Create(_ context.Context, v Voucher) (Voucher, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v.ID == "" {
		v.ID = newID()
	}
	for _, existing := range r.vouchers {
		if existing.Code == v.Code {
			return Voucher{}, fmt.Errorf("voucher code %q already exists", v.Code)
		}
	}
	r.vouchers[v.ID] = v
	return v, nil
}

func (r *InMemoryVoucherRepository) GetByCode(_ context.Context, code string) (Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.vouchers {
		if v.Code == code {
			return v, nil
		}
	}
	return Voucher{}, ErrVoucherNotFound
}

func (r *InMemoryVoucherRepository) GetByID(_ context.Context, id string) (Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.vouchers[id]
	if !ok {
		return Voucher{}, ErrVoucherNotFound
	}
	return v, nil
}

func (r *InMemoryVoucherRepository) List(_ context.Context, sellerID string, limit, offset int) ([]Voucher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []Voucher
	for _, v := range r.vouchers {
		if v.SellerID == sellerID {
			all = append(all, v)
		}
	}
	// Deterministic order (by code) so pagination is stable in the DB-less path.
	sort.Slice(all, func(i, j int) bool { return all[i].Code < all[j].Code })
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryVoucherRepository) IncrementUsed(_ context.Context, id string, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.vouchers[id]
	if !ok {
		return ErrVoucherNotFound
	}
	v.Used += delta
	r.vouchers[id] = v
	return nil
}

func (r *InMemoryVoucherRepository) Ping(_ context.Context) error { return nil }
