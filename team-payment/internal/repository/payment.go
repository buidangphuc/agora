package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTransactionNotFound = errors.New("payment transaction not found")

type PaymentMethod int32

const (
	PaymentMethodUnspecified PaymentMethod = 0
	PaymentMethodCOD         PaymentMethod = 1
	PaymentMethodMockMoMo    PaymentMethod = 2
	PaymentMethodMockBank    PaymentMethod = 3
	PaymentMethodMockCard    PaymentMethod = 4
)

type PaymentStatus int32

const (
	PaymentStatusUnspecified PaymentStatus = 0
	PaymentStatusPending     PaymentStatus = 1
	PaymentStatusPaid        PaymentStatus = 2
	PaymentStatusFailed      PaymentStatus = 3
	PaymentStatusRefunded    PaymentStatus = 4
)

type PaymentTransaction struct {
	ID                string
	OrderID           string
	BuyerID           string
	Amount            int64
	Currency          string
	Method            PaymentMethod
	Status            PaymentStatus
	ProviderReference string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PaymentRepository interface {
	CreateTransaction(ctx context.Context, tx PaymentTransaction) (PaymentTransaction, error)
	GetTransaction(ctx context.Context, id string) (PaymentTransaction, error)
	GetTransactionByOrderID(ctx context.Context, orderID string) (PaymentTransaction, error)
	UpdateStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error)
	UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresPaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresPaymentRepository(pool *pgxpool.Pool) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{pool: pool}
}

const txColumns = `id, order_id, buyer_id, amount, currency, method, status, provider_reference, created_at, updated_at`

func scanTransaction(row pgx.Row, t *PaymentTransaction) error {
	var methodInt, statusInt int32
	if err := row.Scan(&t.ID, &t.OrderID, &t.BuyerID, &t.Amount, &t.Currency, &methodInt, &statusInt, &t.ProviderReference, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return err
	}
	t.Method = PaymentMethod(methodInt)
	t.Status = PaymentStatus(statusInt)
	return nil
}

func (r *PostgresPaymentRepository) CreateTransaction(ctx context.Context, t PaymentTransaction) (PaymentTransaction, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Currency == "" {
		t.Currency = "VND"
	}
	if t.Status == 0 {
		t.Status = PaymentStatusPending
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()

	const q = `INSERT INTO payment_transactions (id, order_id, buyer_id, amount, currency, method, status, provider_reference, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	if _, err := r.pool.Exec(ctx, q, t.ID, t.OrderID, t.BuyerID, t.Amount, t.Currency, int32(t.Method), int32(t.Status), t.ProviderReference, t.CreatedAt, t.UpdatedAt); err != nil {
		return PaymentTransaction{}, fmt.Errorf("insert payment tx: %w", err)
	}
	return t, nil
}

func (r *PostgresPaymentRepository) GetTransaction(ctx context.Context, id string) (PaymentTransaction, error) {
	const q = `SELECT ` + txColumns + ` FROM payment_transactions WHERE id = $1`
	var t PaymentTransaction
	if err := scanTransaction(r.pool.QueryRow(ctx, q, id), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentTransaction{}, ErrTransactionNotFound
		}
		return PaymentTransaction{}, fmt.Errorf("get payment tx: %w", err)
	}
	return t, nil
}

func (r *PostgresPaymentRepository) GetTransactionByOrderID(ctx context.Context, orderID string) (PaymentTransaction, error) {
	const q = `SELECT ` + txColumns + ` FROM payment_transactions WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`
	var t PaymentTransaction
	if err := scanTransaction(r.pool.QueryRow(ctx, q, orderID), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentTransaction{}, ErrTransactionNotFound
		}
		return PaymentTransaction{}, fmt.Errorf("get payment tx by order: %w", err)
	}
	return t, nil
}

func (r *PostgresPaymentRepository) UpdateStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	const q = `UPDATE payment_transactions SET status = $1, provider_reference = $2, updated_at = NOW() WHERE id = $3
		RETURNING ` + txColumns
	var t PaymentTransaction
	if err := scanTransaction(r.pool.QueryRow(ctx, q, int32(status), providerRef, id), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentTransaction{}, ErrTransactionNotFound
		}
		return PaymentTransaction{}, fmt.Errorf("update payment status: %w", err)
	}
	return t, nil
}

func (r *PostgresPaymentRepository) UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return r.UpdateStatus(ctx, id, status, providerRef)
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryPaymentRepository struct {
	mu   sync.RWMutex
	data map[string]PaymentTransaction
}

func NewInMemoryPaymentRepository() *InMemoryPaymentRepository {
	return &InMemoryPaymentRepository{
		data: make(map[string]PaymentTransaction),
	}
}

func (r *InMemoryPaymentRepository) CreateTransaction(_ context.Context, t PaymentTransaction) (PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Currency == "" {
		t.Currency = "VND"
	}
	if t.Status == 0 {
		t.Status = PaymentStatusPending
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	r.data[t.ID] = t
	return t, nil
}

func (r *InMemoryPaymentRepository) GetTransaction(_ context.Context, id string) (PaymentTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.data[id]
	if !ok {
		return PaymentTransaction{}, ErrTransactionNotFound
	}
	return t, nil
}

func (r *InMemoryPaymentRepository) GetTransactionByOrderID(_ context.Context, orderID string) (PaymentTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.data {
		if t.OrderID == orderID {
			return t, nil
		}
	}
	return PaymentTransaction{}, ErrTransactionNotFound
}

func (r *InMemoryPaymentRepository) UpdateStatus(_ context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.data[id]
	if !ok {
		return PaymentTransaction{}, ErrTransactionNotFound
	}
	t.Status = status
	t.ProviderReference = providerRef
	t.UpdatedAt = time.Now()
	r.data[id] = t
	return t, nil
}

func (r *InMemoryPaymentRepository) UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return r.UpdateStatus(ctx, id, status, providerRef)
}
