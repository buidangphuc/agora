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
	ErrReturnNotFound = errors.New("order return not found")
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
	CreateReturn(ctx context.Context, ret OrderReturn) (OrderReturn, error)
	GetReturn(ctx context.Context, id string) (OrderReturn, error)
	GetReturnByOrderID(ctx context.Context, orderID string) (OrderReturn, error)
	ListReturnsByBuyer(ctx context.Context, buyerID string) ([]OrderReturn, error)
	ListReturnsBySeller(ctx context.Context, sellerID string) ([]OrderReturn, error)
	UpdateReturnStatus(ctx context.Context, id string, status ReturnStatus) (OrderReturn, error)
}

type PostgresReturnRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresReturnRepository(pool *pgxpool.Pool) *PostgresReturnRepository {
	return &PostgresReturnRepository{pool: pool}
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

func (r *PostgresReturnRepository) CreateReturn(ctx context.Context, ret OrderReturn) (OrderReturn, error) {
	if ret.ID == "" {
		ret.ID = uuid.NewString()
	}
	if ret.Status == 0 {
		ret.Status = ReturnStatusPending
	}
	ret.CreatedAt = time.Now()
	ret.UpdatedAt = time.Now()

	const q = `INSERT INTO order_returns (` + returnColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if _, err := r.pool.Exec(ctx, q, ret.ID, ret.OrderID, ret.BuyerID, ret.SellerID, ret.Reason, ret.RefundAmount, int32(ret.Status), ret.CreatedAt, ret.UpdatedAt); err != nil {
		return OrderReturn{}, fmt.Errorf("insert return request: %w", err)
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

func (r *PostgresReturnRepository) ListReturnsByBuyer(ctx context.Context, buyerID string) ([]OrderReturn, error) {
	const q = `SELECT ` + returnColumns + ` FROM order_returns WHERE buyer_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, buyerID)
	if err != nil {
		return nil, fmt.Errorf("list returns by buyer %q: %w", buyerID, err)
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

func (r *PostgresReturnRepository) ListReturnsBySeller(ctx context.Context, sellerID string) ([]OrderReturn, error) {
	const q = `SELECT ` + returnColumns + ` FROM order_returns WHERE seller_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, sellerID)
	if err != nil {
		return nil, fmt.Errorf("list returns by seller %q: %w", sellerID, err)
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

func (r *PostgresReturnRepository) UpdateReturnStatus(ctx context.Context, id string, status ReturnStatus) (OrderReturn, error) {
	const q = `UPDATE order_returns SET status = $2, updated_at = now() WHERE id = $1 RETURNING ` + returnColumns
	var ret OrderReturn
	if err := scanReturn(r.pool.QueryRow(ctx, q, id, int32(status)), &ret); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return OrderReturn{}, ErrReturnNotFound
		}
		return OrderReturn{}, fmt.Errorf("update return status %q: %w", id, err)
	}
	return ret, nil
}

// InMemoryReturnRepository
type InMemoryReturnRepository struct {
	mu      sync.RWMutex
	returns map[string]OrderReturn
}

func NewInMemoryReturnRepository() *InMemoryReturnRepository {
	return &InMemoryReturnRepository{
		returns: make(map[string]OrderReturn),
	}
}

func (r *InMemoryReturnRepository) CreateReturn(_ context.Context, ret OrderReturn) (OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ret.ID == "" {
		ret.ID = uuid.NewString()
	}
	if ret.Status == 0 {
		ret.Status = ReturnStatusPending
	}
	ret.CreatedAt = time.Now()
	ret.UpdatedAt = time.Now()

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

func (r *InMemoryReturnRepository) ListReturnsByBuyer(_ context.Context, buyerID string) ([]OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []OrderReturn
	for _, ret := range r.returns {
		if ret.BuyerID == buyerID {
			res = append(res, ret)
		}
	}
	return res, nil
}

func (r *InMemoryReturnRepository) ListReturnsBySeller(_ context.Context, sellerID string) ([]OrderReturn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []OrderReturn
	for _, ret := range r.returns {
		if ret.SellerID == sellerID {
			res = append(res, ret)
		}
	}
	return res, nil
}

func (r *InMemoryReturnRepository) UpdateReturnStatus(_ context.Context, id string, status ReturnStatus) (OrderReturn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ret, ok := r.returns[id]
	if !ok {
		return OrderReturn{}, ErrReturnNotFound
	}
	ret.Status = status
	ret.UpdatedAt = time.Now()
	r.returns[id] = ret
	return ret, nil
}
