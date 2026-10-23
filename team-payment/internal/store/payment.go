package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTransactionNotFound = errors.New("payment transaction not found")

// ErrNotSettleable: a status write found the payment no longer PENDING or FAILED
// (it was paid or refunded meanwhile). Only PENDING/FAILED payments are settled
// or failed, so a refunded payment is never reopened, even by a racing call.
var ErrNotSettleable = errors.New("payment is no longer pending or failed")

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
	// PaymentStatusPartiallyRefunded: 0 < RefundedAmount < Amount (more refunds possible).
	PaymentStatusPartiallyRefunded PaymentStatus = 5
	// PaymentStatusCancelled: voided because its order was cancelled unpaid (terminal).
	PaymentStatusCancelled PaymentStatus = 6
)

// CancelUnpaidOutcome reports what CancelUnpaid found for the order.
type CancelUnpaidOutcome int

const (
	// CancelAbsent: the order has no transaction.
	CancelAbsent CancelUnpaidOutcome = iota
	// CancelVoided: a PENDING or FAILED transaction was moved to CANCELLED.
	CancelVoided
	// CancelAlreadyCancelled: the transaction was already CANCELLED (redelivery).
	CancelAlreadyCancelled
	// CancelOtherStatus: the transaction is settled (paid or refunded); untouched.
	CancelOtherStatus
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
	// RefundedAmount is the cumulative applied amount of the payment's refunds
	// (0..Amount; the sum of its payment_refunds rows, LEGACY included).
	RefundedAmount int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// Refunds are the payment's refunds, oldest first. Filled by the service for the
	// RPC responses; the payment repositories never read or write it.
	Refunds []Refund
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
	return updatePaymentStatusTx(ctx, r.pool, id, status, providerRef)
}

func (r *PostgresPaymentRepository) UpdateTransactionStatus(ctx context.Context, id string, status PaymentStatus, providerRef string) (PaymentTransaction, error) {
	return r.UpdateStatus(ctx, id, status, providerRef)
}

// CancelUnpaid voids the order's transaction: compare-and-set PENDING|FAILED ->
// CANCELLED. It writes no outbox row. On 0 rows it re-reads to report why.
func (r *PostgresPaymentRepository) CancelUnpaid(ctx context.Context, orderID string) (CancelUnpaidOutcome, error) {
	const upd = `UPDATE payment_transactions SET status = 6, updated_at = NOW()
		WHERE order_id = $1 AND status IN (1, 3)`
	tag, err := r.pool.Exec(ctx, upd, orderID)
	if err != nil {
		return CancelAbsent, fmt.Errorf("cancel unpaid payment: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return CancelVoided, nil
	}
	t, err := r.GetTransactionByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, ErrTransactionNotFound) {
			return CancelAbsent, nil
		}
		return CancelAbsent, err
	}
	if t.Status == PaymentStatusCancelled {
		return CancelAlreadyCancelled, nil
	}
	return CancelOtherStatus, nil
}
