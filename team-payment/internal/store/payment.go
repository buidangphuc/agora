package store

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

// ErrTransactionNotFound is returned for: transaction not found.
var ErrTransactionNotFound = errors.New("payment transaction not found")

// ErrNotSettleable means a status write found the payment no longer PENDING or FAILED
// (it was paid or refunded meanwhile). Only PENDING/FAILED payments are settled
// or failed, so a refunded payment is never reopened, even by a racing call.
var ErrNotSettleable = errors.New("payment is no longer pending or failed")

// PaymentMethod is the payment method type.
type PaymentMethod int32

const (
	// PaymentMethodUnspecified is the payment method unspecified value.
	PaymentMethodUnspecified PaymentMethod = 0
	// PaymentMethodCOD is the payment method cod value.
	PaymentMethodCOD PaymentMethod = 1
	// PaymentMethodMockMoMo is the payment method mock mo mo value.
	PaymentMethodMockMoMo PaymentMethod = 2
	// PaymentMethodMockBank is the payment method mock bank value.
	PaymentMethodMockBank PaymentMethod = 3
	// PaymentMethodMockCard is the payment method mock card value.
	PaymentMethodMockCard PaymentMethod = 4
)

// PaymentStatus is the payment status type.
type PaymentStatus int32

const (
	// PaymentStatusUnspecified is the payment status unspecified value.
	PaymentStatusUnspecified PaymentStatus = 0
	// PaymentStatusPending is the payment status pending value.
	PaymentStatusPending PaymentStatus = 1
	// PaymentStatusPaid is the payment status paid value.
	PaymentStatusPaid PaymentStatus = 2
	// PaymentStatusFailed is the payment status failed value.
	PaymentStatusFailed PaymentStatus = 3
	// PaymentStatusRefunded is the payment status refunded value.
	PaymentStatusRefunded PaymentStatus = 4
	// PaymentStatusPartiallyRefunded means 0 < RefundedAmount < Amount (more refunds possible).
	PaymentStatusPartiallyRefunded PaymentStatus = 5
)

// PaymentTransaction is the payment transaction type.
type PaymentTransaction struct {
	ID                string
	OrderID           string
	BuyerID           string
	Amount            int64
	Currency          string
	Method            PaymentMethod
	Status            PaymentStatus
	ProviderReference string
	// RefundedAmount is the cumulative applied amount of the payment's refunds
	// (0..Amount; the sum of its payment_refunds rows, LEGACY included).
	RefundedAmount int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// Refunds are the payment's refunds, oldest first. Filled by the service for the
	// RPC responses; the payment repositories never read or write it.
	Refunds []Refund
}

// PaymentRepository is the payment repository type.
type PaymentRepository interface {
	CreateTransaction(ctx context.Context, tx PaymentTransaction) (PaymentTransaction, error)
	GetTransaction(ctx context.Context, id string) (PaymentTransaction, error)
	GetTransactionByOrderID(ctx context.Context, orderID string) (PaymentTransaction, error)
	UpdateStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error)
	UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

// PostgresPaymentRepository is the postgres payment repository type.
type PostgresPaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresPaymentRepository builds the postgres payment repository.
func NewPostgresPaymentRepository(pool *pgxpool.Pool) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{pool: pool}
}

const txColumns = `id, order_id, buyer_id, amount, currency, method, status, provider_reference, refunded_amount, created_at, updated_at`

func scanTransaction(row pgx.Row, t *PaymentTransaction) error {
	var methodInt, statusInt int32
	if err := row.Scan(&t.ID, &t.OrderID, &t.BuyerID, &t.Amount, &t.Currency, &methodInt, &statusInt, &t.ProviderReference, &t.RefundedAmount, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return err
	}
	t.Method = PaymentMethod(methodInt)
	t.Status = PaymentStatus(statusInt)
	return nil
}

// CreateTransaction implements the postgres payment repository operation "create transaction" over Postgres.
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

// GetTransaction implements the postgres payment repository operation "get transaction" over Postgres.
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

// GetTransactionByOrderID implements the postgres payment repository operation "get transaction by order id" over Postgres.
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

// UpdateStatus implements the postgres payment repository operation "update status" over Postgres.
func (r *PostgresPaymentRepository) UpdateStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return updatePaymentStatusTx(ctx, r.pool, id, status, providerRef)
}

// UpdateTransactionStatus implements the postgres payment repository operation "update transaction status" over Postgres.
func (r *PostgresPaymentRepository) UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return r.UpdateStatus(ctx, id, status, providerRef)
}

// ── InMemory Implementation ───────────────────────────────────────────

// InMemoryPaymentRepository is the in memory payment repository type.
type InMemoryPaymentRepository struct {
	mu   sync.RWMutex
	data map[string]PaymentTransaction
}

// NewInMemoryPaymentRepository builds the in memory payment repository.
func NewInMemoryPaymentRepository() *InMemoryPaymentRepository {
	return &InMemoryPaymentRepository{
		data: make(map[string]PaymentTransaction),
	}
}

// CreateTransaction implements "create transaction" in memory (tests).
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

// GetTransaction implements "get transaction" in memory (tests).
func (r *InMemoryPaymentRepository) GetTransaction(_ context.Context, id string) (PaymentTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.data[id]
	if !ok {
		return PaymentTransaction{}, ErrTransactionNotFound
	}
	return t, nil
}

// GetTransactionByOrderID implements "get transaction by order id" in memory (tests).
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

// UpdateStatus implements "update status" in memory (tests).
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

// UpdateTransactionStatus implements "update transaction status" in memory (tests).
func (r *InMemoryPaymentRepository) UpdateTransactionStatus(_ context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return r.updateStatusFromSettleable(id, status, providerRef)
}

// updateStatusFromSettleable is the compare-and-set the Postgres writer runs:
// the write applies only to a PENDING or FAILED payment.
func (r *InMemoryPaymentRepository) updateStatusFromSettleable(id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.data[id]
	if !ok {
		return PaymentTransaction{}, ErrTransactionNotFound
	}
	if t.Status != PaymentStatusPending && t.Status != PaymentStatusFailed {
		return PaymentTransaction{}, ErrNotSettleable
	}
	t.Status = status
	t.ProviderReference = providerRef
	t.UpdatedAt = time.Now()
	r.data[id] = t
	return t, nil
}
