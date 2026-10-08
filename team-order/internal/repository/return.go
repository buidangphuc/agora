package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrReturnNotFound = errors.New("order return not found")
	// ErrReturnStatusConflict is returned by TransitionReturn when the return
	// exists but is no longer in the expected source status at the moment of the
	// write (a lost compare-and-set).
	ErrReturnStatusConflict = errors.New("return status changed concurrently")
	// ErrReturnExceedsRemainder is returned by CreateReturnCapped when the
	// requested amount is above the order's returnable remainder.
	ErrReturnExceedsRemainder = errors.New("refund amount exceeds the order's returnable remainder")
	// ErrNoReturnableRemainder is returned by CreateReturnCapped for a request
	// without an amount when the order's non-rejected returns already cover its total.
	ErrNoReturnableRemainder = errors.New("order has no returnable amount left")
)

type ReturnStatus int32

const (
	ReturnStatusUnspecified ReturnStatus = 0
	ReturnStatusPending     ReturnStatus = 1
	ReturnStatusApproved    ReturnStatus = 2
	ReturnStatusRejected    ReturnStatus = 3
	ReturnStatusRefunded    ReturnStatus = 4
)

type OrderReturn struct {
	ID           string
	OrderID      string
	BuyerID      string
	SellerID     string
	Reason       string
	RefundAmount int64
	Status       ReturnStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ReturnRepository interface {
	// CreateReturn inserts a return without the per-order cap (fixtures only; the
	// service uses CreateReturnCapped).
	CreateReturn(ctx context.Context, ret OrderReturn) (OrderReturn, error)
	// CreateReturnCapped inserts ret only if its RefundAmount fits the order's
	// returnable remainder: orderTotal minus the refund amounts of the order's
	// returns that are not REJECTED. A RefundAmount <= 0 means "the remainder".
	// The check and the insert are serialised per order (the order row is locked
	// FOR UPDATE in Postgres, the repository mutex in memory). It returns
	// ErrReturnExceedsRemainder, ErrNoReturnableRemainder or ErrOrderNotFound.
	// orderTotal is the order's total_amount, which never changes after placement.
	CreateReturnCapped(ctx context.Context, ret OrderReturn, orderTotal int64) (OrderReturn, error)
	GetReturn(ctx context.Context, id string) (OrderReturn, error)
	GetReturnByOrderID(ctx context.Context, orderID string) (OrderReturn, error)
	ListReturnsByBuyer(ctx context.Context, buyerID string) ([]OrderReturn, error)
	ListReturnsBySeller(ctx context.Context, sellerID string) ([]OrderReturn, error)
	// ListReturnsByOrder lists all returns of an order, newest first.
	ListReturnsByOrder(ctx context.Context, orderID string) ([]OrderReturn, error)
	// TransitionReturn is the only return status write: a compare-and-set that
	// moves the return from `from` to `to` only if its status at the moment of the
	// write is `from`. It returns ErrReturnNotFound or ErrReturnStatusConflict
	// otherwise. A won APPROVED -> REFUNDED writes the ReturnRefunded outbox row
	// (WithReturnOutbox) in the same transaction; nothing else writes one.
	TransitionReturn(ctx context.Context, id string, from, to ReturnStatus) (OrderReturn, error)
}

// ReturnOutboxBuilder turns a return that has just won APPROVED -> REFUNDED into
// the outbox row to persist alongside that transition (ADR-0013). ret is the row
// the compare-and-set returned (its RefundAmount is the stored amount and its
// UpdatedAt the transition time); currency is the order's currency. Injected by
// internal/events so the repository stays free of an events dependency.
type ReturnOutboxBuilder func(ret OrderReturn, currency string) (OutboxRow, error)

// ReturnRepoOption customizes a return repository.
type ReturnRepoOption func(*returnRepoConfig)

type returnRepoConfig struct {
	refundedOutbox ReturnOutboxBuilder
	outbox         OutboxRepository // in-memory repo only
	orders         OrderReader      // in-memory repo only: the order currency
}

// WithReturnOutbox makes every won APPROVED -> REFUNDED transition write an
// outbox row built by b in the SAME transaction as the status update. A lost
// compare-and-set or any other transition writes nothing; if building or
// enqueueing the row fails, the transition is rolled back too.
func WithReturnOutbox(b ReturnOutboxBuilder) ReturnRepoOption {
	return func(c *returnRepoConfig) { c.refundedOutbox = b }
}

// WithInMemoryReturnOutbox sets the outbox store the in-memory repository writes
// to, atomically with the transition, and the order store it reads the currency
// from. Ignored by the Postgres repository.
func WithInMemoryReturnOutbox(o OutboxRepository, orders OrderReader) ReturnRepoOption {
	return func(c *returnRepoConfig) { c.outbox = o; c.orders = orders }
}

func newReturnRepoConfig(opts []ReturnRepoOption) returnRepoConfig {
	var c returnRepoConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// returnRemainder applies the cap rule to the sum of the order's non-rejected
// returns, returning the amount to store. A positive amount above the remainder
// (including a remainder of 0) is ErrReturnExceedsRemainder; a defaulted amount
// with nothing left is ErrNoReturnableRemainder.
func returnRemainder(requested, orderTotal, taken int64) (int64, error) {
	remaining := orderTotal - taken
	if remaining < 0 {
		remaining = 0
	}
	if requested <= 0 {
		if remaining == 0 {
			return 0, ErrNoReturnableRemainder
		}
		return remaining, nil
	}
	if requested > remaining {
		return 0, fmt.Errorf("%w: requested %d, remaining %d", ErrReturnExceedsRemainder, requested, remaining)
	}
	return requested, nil
}

type PostgresReturnRepository struct {
	pool           *pgxpool.Pool
	refundedOutbox ReturnOutboxBuilder
}

func NewPostgresReturnRepository(pool *pgxpool.Pool, opts ...ReturnRepoOption) *PostgresReturnRepository {
	cfg := newReturnRepoConfig(opts)
	return &PostgresReturnRepository{pool: pool, refundedOutbox: cfg.refundedOutbox}
}

const returnColumns = `id, order_id, buyer_id, seller_id, reason, refund_amount, status, created_at, updated_at`

func scanReturn(row pgx.Row, r *OrderReturn) error {
	var statusInt int32
	if err := row.Scan(&r.ID, &r.OrderID, &r.BuyerID, &r.SellerID, &r.Reason, &r.RefundAmount, &statusInt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return err
	}
	r.Status = ReturnStatus(statusInt)
	return nil
}

func prepareReturn(ret *OrderReturn) {
	if ret.ID == "" {
		ret.ID = uuid.NewString()
	}
	if ret.Status == 0 {
		ret.Status = ReturnStatusPending
	}
	ret.CreatedAt = time.Now()
	ret.UpdatedAt = ret.CreatedAt
}

const insertReturnSQL = `INSERT INTO order_returns (` + returnColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

func (r *PostgresReturnRepository) CreateReturn(ctx context.Context, ret OrderReturn) (OrderReturn, error) {
	prepareReturn(&ret)
	if _, err := r.pool.Exec(ctx, insertReturnSQL, ret.ID, ret.OrderID, ret.BuyerID, ret.SellerID, ret.Reason, ret.RefundAmount, int32(ret.Status), ret.CreatedAt, ret.UpdatedAt); err != nil {
		return OrderReturn{}, fmt.Errorf("insert return request: %w", err)
	}
	return ret, nil
}

func (r *PostgresReturnRepository) CreateReturnCapped(ctx context.Context, ret OrderReturn, orderTotal int64) (OrderReturn, error) {
	prepareReturn(&ret)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// The order row lock serialises every capped create of this order, so the sum
	// below always sees the returns committed before it.
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM orders WHERE id = $1 FOR UPDATE`, ret.OrderID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderReturn{}, ErrOrderNotFound
		}
		return OrderReturn{}, fmt.Errorf("lock order %q: %w", ret.OrderID, err)
	}
	var taken int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(refund_amount), 0) FROM order_returns WHERE order_id = $1 AND status <> $2`,
		ret.OrderID, int32(ReturnStatusRejected)).Scan(&taken); err != nil {
		return OrderReturn{}, fmt.Errorf("sum returns of order %q: %w", ret.OrderID, err)
	}
	amount, err := returnRemainder(ret.RefundAmount, orderTotal, taken)
	if err != nil {
		return OrderReturn{}, err
	}
	ret.RefundAmount = amount
	if _, err := tx.Exec(ctx, insertReturnSQL, ret.ID, ret.OrderID, ret.BuyerID, ret.SellerID, ret.Reason, ret.RefundAmount, int32(ret.Status), ret.CreatedAt, ret.UpdatedAt); err != nil {
		return OrderReturn{}, fmt.Errorf("insert return request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderReturn{}, fmt.Errorf("commit tx: %w", err)
	}
	return ret, nil
}

func (r *PostgresReturnRepository) GetReturn(ctx context.Context, id string) (OrderReturn, error) {
	const q = `SELECT ` + returnColumns + ` FROM order_returns WHERE id = $1`
	var ret OrderReturn
	if err := scanReturn(r.pool.QueryRow(ctx, q, id), &ret); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return OrderReturn{}, ErrReturnNotFound
		}
		return OrderReturn{}, fmt.Errorf("get return %q: %w", id, err)
	}
	return ret, nil
}

func (r *PostgresReturnRepository) GetReturnByOrderID(ctx context.Context, orderID string) (OrderReturn, error) {
	const q = `SELECT ` + returnColumns + ` FROM order_returns WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`
	var ret OrderReturn
	if err := scanReturn(r.pool.QueryRow(ctx, q, orderID), &ret); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return OrderReturn{}, ErrReturnNotFound
		}
		return OrderReturn{}, fmt.Errorf("get return by order %q: %w", orderID, err)
	}
	return ret, nil
}

func (r *PostgresReturnRepository) listReturns(ctx context.Context, q, arg, what string) ([]OrderReturn, error) {
	rows, err := r.pool.Query(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("list returns by %s %q: %w", what, arg, err)
	}
	defer rows.Close()

	var returns []OrderReturn
	for rows.Next() {
		var ret OrderReturn
		if err := scanReturn(rows, &ret); err != nil {
			return nil, err
		}
		returns = append(returns, ret)
	}
	return returns, rows.Err()
}

func (r *PostgresReturnRepository) ListReturnsByBuyer(ctx context.Context, buyerID string) ([]OrderReturn, error) {
	return r.listReturns(ctx, `SELECT `+returnColumns+` FROM order_returns WHERE buyer_id = $1 ORDER BY created_at DESC`, buyerID, "buyer")
}

func (r *PostgresReturnRepository) ListReturnsBySeller(ctx context.Context, sellerID string) ([]OrderReturn, error) {
	return r.listReturns(ctx, `SELECT `+returnColumns+` FROM order_returns WHERE seller_id = $1 ORDER BY created_at DESC`, sellerID, "seller")
}

func (r *PostgresReturnRepository) ListReturnsByOrder(ctx context.Context, orderID string) ([]OrderReturn, error) {
	return r.listReturns(ctx, `SELECT `+returnColumns+` FROM order_returns WHERE order_id = $1 ORDER BY created_at DESC, id DESC`, orderID, "order")
}

func (r *PostgresReturnRepository) TransitionReturn(ctx context.Context, id string, from, to ReturnStatus) (OrderReturn, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// The status condition is decided by this UPDATE itself (row lock), never by
	// an earlier read: of concurrent transitions of one return, one wins.
	const q = `UPDATE order_returns SET status = $3, updated_at = now()
		WHERE id = $1 AND status = $2 RETURNING ` + returnColumns
	var ret OrderReturn
	if err := scanReturn(tx.QueryRow(ctx, q, id, int32(from), int32(to)), &ret); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if qerr := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM order_returns WHERE id = $1)`, id).Scan(&exists); qerr != nil {
				return OrderReturn{}, fmt.Errorf("check return %q: %w", id, qerr)
			}
			if !exists {
				return OrderReturn{}, ErrReturnNotFound
			}
			return OrderReturn{}, ErrReturnStatusConflict
		}
		return OrderReturn{}, fmt.Errorf("transition return %q: %w", id, err)
	}

	// Domain fact (ADR-0013): ReturnRefunded commits or rolls back with the won
	// APPROVED -> REFUNDED compare-and-set, so it is written once per return.
	if from == ReturnStatusApproved && to == ReturnStatusRefunded && r.refundedOutbox != nil {
		var currency string
		if err := tx.QueryRow(ctx, `SELECT currency FROM orders WHERE id = $1`, ret.OrderID).Scan(&currency); err != nil {
			return OrderReturn{}, fmt.Errorf("read currency of order %q: %w", ret.OrderID, err)
		}
		row, err := r.refundedOutbox(ret, currency)
		if err != nil {
			return OrderReturn{}, fmt.Errorf("build return refunded outbox row: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, row); err != nil {
			return OrderReturn{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderReturn{}, fmt.Errorf("commit tx: %w", err)
	}
	return ret, nil
}

// InMemoryReturnRepository
type InMemoryReturnRepository struct {
	mu             sync.RWMutex
	returns        map[string]OrderReturn
	refundedOutbox ReturnOutboxBuilder
	outbox         OutboxRepository
	orders         OrderReader
}

func NewInMemoryReturnRepository(opts ...ReturnRepoOption) *InMemoryReturnRepository {
	cfg := newReturnRepoConfig(opts)
	return &InMemoryReturnRepository{
		returns:        make(map[string]OrderReturn),
		refundedOutbox: cfg.refundedOutbox,
		outbox:         cfg.outbox,
		orders:         cfg.orders,
	}
}

func (r *InMemoryReturnRepository) CreateReturn(_ context.Context, ret OrderReturn) (OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prepareReturn(&ret)
	r.returns[ret.ID] = ret
	return ret, nil
}

func (r *InMemoryReturnRepository) CreateReturnCapped(_ context.Context, ret OrderReturn, orderTotal int64) (OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var taken int64
	for _, existing := range r.returns {
		if existing.OrderID == ret.OrderID && existing.Status != ReturnStatusRejected {
			taken += existing.RefundAmount
		}
	}
	amount, err := returnRemainder(ret.RefundAmount, orderTotal, taken)
	if err != nil {
		return OrderReturn{}, err
	}
	ret.RefundAmount = amount
	prepareReturn(&ret)
	r.returns[ret.ID] = ret
	return ret, nil
}

func (r *InMemoryReturnRepository) GetReturn(_ context.Context, id string) (OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ret, ok := r.returns[id]
	if !ok {
		return OrderReturn{}, ErrReturnNotFound
	}
	return ret, nil
}

func (r *InMemoryReturnRepository) GetReturnByOrderID(_ context.Context, orderID string) (OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ret := range r.returns {
		if ret.OrderID == orderID {
			return ret, nil
		}
	}
	return OrderReturn{}, ErrReturnNotFound
}

func (r *InMemoryReturnRepository) filter(keep func(OrderReturn) bool) []OrderReturn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []OrderReturn
	for _, ret := range r.returns {
		if keep(ret) {
			res = append(res, ret)
		}
	}
	return res
}

func (r *InMemoryReturnRepository) ListReturnsByBuyer(_ context.Context, buyerID string) ([]OrderReturn, error) {
	return r.filter(func(ret OrderReturn) bool { return ret.BuyerID == buyerID }), nil
}

func (r *InMemoryReturnRepository) ListReturnsBySeller(_ context.Context, sellerID string) ([]OrderReturn, error) {
	return r.filter(func(ret OrderReturn) bool { return ret.SellerID == sellerID }), nil
}

func (r *InMemoryReturnRepository) ListReturnsByOrder(_ context.Context, orderID string) ([]OrderReturn, error) {
	res := r.filter(func(ret OrderReturn) bool { return ret.OrderID == orderID })
	sort.Slice(res, func(i, j int) bool {
		if !res[i].CreatedAt.Equal(res[j].CreatedAt) {
			return res[i].CreatedAt.After(res[j].CreatedAt)
		}
		return res[i].ID > res[j].ID
	})
	return res, nil
}

func (r *InMemoryReturnRepository) TransitionReturn(ctx context.Context, id string, from, to ReturnStatus) (OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ret, ok := r.returns[id]
	if !ok {
		return OrderReturn{}, ErrReturnNotFound
	}
	if ret.Status != from {
		return OrderReturn{}, ErrReturnStatusConflict
	}
	ret.Status = to
	ret.UpdatedAt = time.Now()
	// Mirror the Postgres repo: the outbox row and the transition are one unit;
	// the return is stored only after the row is enqueued.
	if from == ReturnStatusApproved && to == ReturnStatusRefunded && r.refundedOutbox != nil && r.outbox != nil {
		currency := ""
		if r.orders != nil {
			o, err := r.orders.GetOrder(ctx, ret.OrderID)
			if err != nil {
				return OrderReturn{}, fmt.Errorf("read currency of order %q: %w", ret.OrderID, err)
			}
			currency = o.Currency
		}
		row, err := r.refundedOutbox(ret, currency)
		if err != nil {
			return OrderReturn{}, fmt.Errorf("build return refunded outbox row: %w", err)
		}
		if err := r.outbox.Enqueue(ctx, row); err != nil {
			return OrderReturn{}, err
		}
	}
	r.returns[id] = ret
	return ret, nil
}
