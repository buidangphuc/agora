package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotSettled is returned when an order has no PAID or REFUNDED payment
	// transaction, so there is nothing to credit or refund.
	ErrNotSettled = errors.New("order has no paid or refunded payment transaction")
	// ErrNotRefundable is returned by Refund when the transaction is no longer PAID (the
	// compare-and-set lost: already refunded, or never paid).
	ErrNotRefundable = errors.New("payment transaction is not PAID")
)

// CreditResult reports what CreditSettlement wrote.
type CreditResult struct {
	Transaction PaymentTransaction
	Credit      LedgerEntry
	Credited    bool // the ORDER_SETTLEMENT entry was written by this call
	Deducted    bool // a REFUND_DEDUCTION entry was written by this call
}

// RefundResult reports what Refund wrote.
type RefundResult struct {
	Transaction PaymentTransaction
	Deducted    bool // a REFUND_DEDUCTION entry was written for the credited seller
}

// SettlementLedger moves seller money for a payment: the settlement credit and the
// refund deduction (design D4). Both serialise on the payment row and then take the
// seller lock (the same one payouts take), and both ledger writes are unique per
// (type, payment id), so credit-then-refund and refund-then-credit both end with one
// credit and one deduction, and a payment that was never credited is never deducted.
type SettlementLedger interface {
	// SettledTransaction returns the order's PAID or REFUNDED transaction
	// (ErrNotSettled when none).
	SettledTransaction(ctx context.Context, orderID string) (PaymentTransaction, error)
	// CreditSettlement appends one ORDER_SETTLEMENT for sellerID of the amount of the
	// order's PAID/REFUNDED transaction, referencing it (no-op when it exists). When the
	// transaction is already REFUNDED it also appends the refund deduction for the
	// credited seller. ErrNotSettled when there is no such transaction, ErrInvalidAmount
	// for a non-positive amount.
	CreditSettlement(ctx context.Context, orderID, sellerID string) (CreditResult, error)
	// Refund moves transaction txID PAID -> REFUNDED by compare-and-set, storing amount
	// as refunded_amount and providerRef, and, when the payment was credited, appends one
	// REFUND_DEDUCTION of -amount for the credited seller. ErrNotRefundable when the
	// transaction is not PAID, ErrTransactionNotFound when it does not exist. Never
	// blocked by the seller's balance or hold.
	Refund(ctx context.Context, txID string, amount int64, providerRef string) (RefundResult, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresSettlementLedger struct {
	pool *pgxpool.Pool
}

func NewPostgresSettlementLedger(pool *pgxpool.Pool) *PostgresSettlementLedger {
	return &PostgresSettlementLedger{pool: pool}
}

const settledTxSQL = `SELECT ` + txColumns + ` FROM payment_transactions
	WHERE order_id = $1 AND status IN (2, 4)
	ORDER BY created_at DESC, id DESC LIMIT 1`

func (s *PostgresSettlementLedger) SettledTransaction(ctx context.Context, orderID string) (PaymentTransaction, error) {
	var t PaymentTransaction
	if err := scanTransaction(s.pool.QueryRow(ctx, settledTxSQL, orderID), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentTransaction{}, ErrNotSettled
		}
		return PaymentTransaction{}, fmt.Errorf("load settled payment tx: %w", err)
	}
	return t, nil
}

func (s *PostgresSettlementLedger) CreditSettlement(ctx context.Context, orderID, sellerID string) (CreditResult, error) {
	if sellerID == "" {
		return CreditResult{}, errors.New("seller id is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreditResult{}, fmt.Errorf("begin credit tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The payment row lock orders this credit against a concurrent refund of the same
	// payment (lock order: payment row, then seller lock).
	var t PaymentTransaction
	if err := scanTransaction(tx.QueryRow(ctx, settledTxSQL+` FOR UPDATE`, orderID), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CreditResult{}, ErrNotSettled
		}
		return CreditResult{}, fmt.Errorf("lock settled payment tx: %w", err)
	}
	if t.Amount <= 0 {
		return CreditResult{}, ErrInvalidAmount
	}
	if err := lockSeller(ctx, tx, sellerID); err != nil {
		return CreditResult{}, err
	}
	credit, credited, err := insertLedgerOnce(ctx, tx, LedgerEntry{
		SellerID:    sellerID,
		Type:        LedgerTypeOrderSettlement,
		Amount:      t.Amount,
		Status:      LedgerStatusCompleted,
		ReferenceID: t.ID,
	})
	if err != nil {
		return CreditResult{}, err
	}
	res := CreditResult{Transaction: t, Credit: credit, Credited: credited}
	if t.Status == PaymentStatusRefunded && t.RefundedAmount > 0 {
		// Refund landed before the credit: the credit path books the deduction too, for
		// the seller the credit belongs to.
		if credit.SellerID != sellerID {
			if err := lockSeller(ctx, tx, credit.SellerID); err != nil {
				return CreditResult{}, err
			}
		}
		_, deducted, err := insertLedgerOnce(ctx, tx, refundDeduction(credit.SellerID, t.ID, t.RefundedAmount))
		if err != nil {
			return CreditResult{}, err
		}
		res.Deducted = deducted
	}
	if err := tx.Commit(ctx); err != nil {
		return CreditResult{}, fmt.Errorf("commit credit: %w", err)
	}
	return res, nil
}

func (s *PostgresSettlementLedger) Refund(ctx context.Context, txID string, amount int64, providerRef string) (RefundResult, error) {
	if amount <= 0 {
		return RefundResult{}, ErrInvalidAmount
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RefundResult{}, fmt.Errorf("begin refund tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const cas = `UPDATE payment_transactions
		SET status = 4, refunded_amount = $2, provider_reference = $3, updated_at = NOW()
		WHERE id = $1 AND status = 2
		RETURNING ` + txColumns
	var t PaymentTransaction
	if err := scanTransaction(tx.QueryRow(ctx, cas, txID, amount, providerRef), &t); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return RefundResult{}, fmt.Errorf("refund payment tx: %w", err)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_transactions WHERE id = $1)`, txID).Scan(&exists); err != nil {
			return RefundResult{}, fmt.Errorf("check payment tx: %w", err)
		}
		if !exists {
			return RefundResult{}, ErrTransactionNotFound
		}
		return RefundResult{}, ErrNotRefundable
	}
	res := RefundResult{Transaction: t}
	credit, err := entryByReference(ctx, tx, LedgerTypeOrderSettlement, t.ID)
	switch {
	case errors.Is(err, ErrLedgerEntryNotFound):
		// Not credited (yet): the credit path writes the deduction if it ever credits.
	case err != nil:
		return RefundResult{}, err
	default:
		if err := lockSeller(ctx, tx, credit.SellerID); err != nil {
			return RefundResult{}, err
		}
		_, deducted, err := insertLedgerOnce(ctx, tx, refundDeduction(credit.SellerID, t.ID, amount))
		if err != nil {
			return RefundResult{}, err
		}
		res.Deducted = deducted
	}
	if err := tx.Commit(ctx); err != nil {
		return RefundResult{}, fmt.Errorf("commit refund: %w", err)
	}
	return res, nil
}

func refundDeduction(sellerID, txID string, refunded int64) LedgerEntry {
	return LedgerEntry{
		SellerID:    sellerID,
		Type:        LedgerTypeRefundDeduction,
		Amount:      -refunded,
		Status:      LedgerStatusCompleted,
		ReferenceID: txID,
	}
}

// ── InMemory Implementation ───────────────────────────────────────────

// InMemorySettlementLedger is the in-memory SettlementLedger over the in-memory payment
// and ledger stores. One mutex plays the payment-row lock; the ledger's own mutex plays
// the seller lock (lock order: s.mu, payments.mu, ledger.mu).
type InMemorySettlementLedger struct {
	mu       sync.Mutex
	payments *InMemoryPaymentRepository
	ledger   *InMemoryLedgerRepository
}

func NewInMemorySettlementLedger(payments *InMemoryPaymentRepository, ledger *InMemoryLedgerRepository) *InMemorySettlementLedger {
	return &InMemorySettlementLedger{payments: payments, ledger: ledger}
}

func (s *InMemorySettlementLedger) settledLocked(orderID string) (PaymentTransaction, bool) {
	s.payments.mu.RLock()
	defer s.payments.mu.RUnlock()
	var best PaymentTransaction
	found := false
	for _, t := range s.payments.data {
		if t.OrderID != orderID || (t.Status != PaymentStatusPaid && t.Status != PaymentStatusRefunded) {
			continue
		}
		if !found || t.CreatedAt.After(best.CreatedAt) {
			best, found = t, true
		}
	}
	return best, found
}

func (s *InMemorySettlementLedger) SettledTransaction(_ context.Context, orderID string) (PaymentTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.settledLocked(orderID)
	if !ok {
		return PaymentTransaction{}, ErrNotSettled
	}
	return t, nil
}

func (s *InMemorySettlementLedger) CreditSettlement(_ context.Context, orderID, sellerID string) (CreditResult, error) {
	if sellerID == "" {
		return CreditResult{}, errors.New("seller id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.settledLocked(orderID)
	if !ok {
		return CreditResult{}, ErrNotSettled
	}
	if t.Amount <= 0 {
		return CreditResult{}, ErrInvalidAmount
	}
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	credit, credited, err := s.ledger.appendOnceLocked(LedgerEntry{
		SellerID: sellerID, Type: LedgerTypeOrderSettlement, Amount: t.Amount,
		Status: LedgerStatusCompleted, ReferenceID: t.ID,
	})
	if err != nil {
		return CreditResult{}, err
	}
	res := CreditResult{Transaction: t, Credit: credit, Credited: credited}
	if t.Status == PaymentStatusRefunded && t.RefundedAmount > 0 {
		_, deducted, err := s.ledger.appendOnceLocked(refundDeduction(credit.SellerID, t.ID, t.RefundedAmount))
		if err != nil {
			return CreditResult{}, err
		}
		res.Deducted = deducted
	}
	return res, nil
}

func (s *InMemorySettlementLedger) Refund(_ context.Context, txID string, amount int64, providerRef string) (RefundResult, error) {
	if amount <= 0 {
		return RefundResult{}, ErrInvalidAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.payments.mu.Lock()
	t, ok := s.payments.data[txID]
	switch {
	case !ok:
		s.payments.mu.Unlock()
		return RefundResult{}, ErrTransactionNotFound
	case t.Status != PaymentStatusPaid:
		s.payments.mu.Unlock()
		return RefundResult{}, ErrNotRefundable
	case amount > t.Amount:
		s.payments.mu.Unlock()
		return RefundResult{}, ErrInvalidAmount
	}
	t.Status = PaymentStatusRefunded
	t.RefundedAmount = amount
	t.ProviderReference = providerRef
	t.UpdatedAt = time.Now()
	s.payments.data[txID] = t
	s.payments.mu.Unlock()

	res := RefundResult{Transaction: t}
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	if credit, ok := s.ledger.byReferenceLocked(LedgerTypeOrderSettlement, t.ID); ok {
		_, deducted, err := s.ledger.appendOnceLocked(refundDeduction(credit.SellerID, t.ID, amount))
		if err != nil {
			return RefundResult{}, err
		}
		res.Deducted = deducted
	}
	return res, nil
}

var (
	_ SettlementLedger = (*PostgresSettlementLedger)(nil)
	_ SettlementLedger = (*InMemorySettlementLedger)(nil)
)
