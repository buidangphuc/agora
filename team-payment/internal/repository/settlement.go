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
	// ErrNotSettled is returned when an order has no PAID, PARTIALLY_REFUNDED or REFUNDED
	// payment transaction, so there is nothing to credit or refund.
	ErrNotSettled = errors.New("order has no paid or refunded payment transaction")
	// ErrNotRefundable is returned by ApplyRefund when the payment is neither PAID nor
	// PARTIALLY_REFUNDED (already REFUNDED, or never paid).
	ErrNotRefundable = errors.New("payment transaction is not refundable")
	// ErrExceedsRemainder is returned by a strict ApplyRefund whose requested amount is
	// above the payment's refundable remainder (amount - refunded_amount).
	ErrExceedsRemainder = errors.New("refund amount exceeds the refundable remainder")
	// ErrRefundIDConflict is returned when a refund key is already stored for another
	// payment or another requested amount.
	ErrRefundIDConflict = errors.New("refund id already used for another payment or amount")
)

// Refund sources (payment_refunds.source).
const (
	RefundSourceSellerOrAdmin = "SELLER_OR_ADMIN"
	RefundSourceReturn        = "RETURN"
	RefundSourceOrderCancel   = "ORDER_CANCEL"
	RefundSourceLegacy        = "LEGACY"
)

// RefundMode is how ApplyRefund treats a requested amount against the refundable
// remainder (design D1 step 4).
type RefundMode int

const (
	// RefundStrict (RPC): a request above the remainder is ErrExceedsRemainder; a
	// payment that is not PAID / PARTIALLY_REFUNDED is ErrNotRefundable.
	RefundStrict RefundMode = iota + 1
	// RefundClamp (return): applies min(requested, remaining) and records both; a
	// REFUNDED payment records an applied 0 (design D6).
	RefundClamp
	// RefundRemainder (cancel): applies whatever remains; nothing is written when 0
	// remains or the payment is REFUNDED. The requested amount is ignored.
	RefundRemainder
)

// Refund is one payment_refunds row: one applied refund of a payment.
type Refund struct {
	ID              string // stored refund key: rpc:/return:/cancel:/legacy:<id>
	PaymentID       string
	Source          string
	SourceID        string
	RequestedAmount int64
	Amount          int64 // applied
	Reason          string
	CreatedAt       time.Time
}

// RefundRequest is one ApplyRefund call.
type RefundRequest struct {
	PaymentID string
	Key       string // stored refund key (design D2)
	Source    string
	SourceID  string
	Requested int64 // ignored in RefundRemainder mode
	Reason    string
	Mode      RefundMode
}

// CreditResult reports what CreditSettlement wrote.
type CreditResult struct {
	Transaction PaymentTransaction
	Credit      LedgerEntry
	Credited    bool // the ORDER_SETTLEMENT entry was written by this call
	Deducted    bool // at least one REFUND_DEDUCTION entry was written by this call
}

// RefundResult reports what ApplyRefund wrote.
type RefundResult struct {
	Transaction PaymentTransaction // the payment after the call
	Refund      Refund             // the refund row (zero when nothing was written)
	Created     bool               // this call wrote the refund row
	Deducted    bool               // a REFUND_DEDUCTION entry was written for the credited seller
}

// SettlementLedger moves seller money for a payment: the settlement credit and the refund
// deductions (design D1/D3). Every path serialises on the payment row and then takes the
// seller lock (the same one payouts take). The credit is unique per payment and each
// deduction unique per refund key, so credit-then-refunds and refunds-then-credit both end
// with one credit and one deduction per positive refund, and a payment that was never
// credited is never deducted.
type SettlementLedger interface {
	// SettledTransaction returns the order's PAID, PARTIALLY_REFUNDED or REFUNDED
	// transaction (ErrNotSettled when none).
	SettledTransaction(ctx context.Context, orderID string) (PaymentTransaction, error)
	// CreditSettlement appends one ORDER_SETTLEMENT for sellerID of the amount of the
	// order's settled transaction, referencing it (no-op when it exists), then one
	// REFUND_DEDUCTION per refund of the payment with a positive applied amount, each
	// referencing its refund (no-op when it exists). ErrNotSettled when there is no such
	// transaction, ErrInvalidAmount for a non-positive amount.
	CreditSettlement(ctx context.Context, orderID, sellerID string) (CreditResult, error)
	// ApplyRefund applies one refund of a payment in one transaction (design D1): lock the
	// payment row; replay an existing key (same payment and requested amount, or any
	// amount in remainder mode) or refuse it (ErrRefundIDConflict); apply the mode; write
	// the refund row, refunded_amount and the status (REFUNDED when fully refunded,
	// otherwise PARTIALLY_REFUNDED); and, when the payment was credited and the applied
	// amount is positive, one REFUND_DEDUCTION of -applied referencing the key.
	// ErrTransactionNotFound when the payment does not exist. Never blocked by the
	// seller's balance or hold.
	ApplyRefund(ctx context.Context, req RefundRequest) (RefundResult, error)
	// ListRefunds returns the payment's refunds, oldest first.
	ListRefunds(ctx context.Context, paymentID string) ([]Refund, error)
}

func validateRefundRequest(req RefundRequest) error {
	if req.PaymentID == "" || req.Key == "" || req.Source == "" || req.SourceID == "" {
		return errors.New("refund payment id, key, source and source id are required")
	}
	switch req.Mode {
	case RefundStrict, RefundClamp:
		if req.Requested <= 0 {
			return ErrInvalidAmount
		}
	case RefundRemainder:
	default:
		return fmt.Errorf("unknown refund mode %d", req.Mode)
	}
	return nil
}

// replayOf decides what an existing row with the request's key means: a replay (nil) or
// a conflict.
func replayOf(existing Refund, req RefundRequest) error {
	if existing.PaymentID != req.PaymentID {
		return ErrRefundIDConflict
	}
	if req.Mode != RefundRemainder && existing.RequestedAmount != req.Requested {
		return ErrRefundIDConflict
	}
	return nil
}

// plan applies the mode to the locked payment (design D1 steps 3-4). write=false means
// nothing is to be written (remainder mode with nothing left).
func plan(t PaymentTransaction, req RefundRequest) (requested, applied int64, write bool, err error) {
	refundable := t.Status == PaymentStatusPaid || t.Status == PaymentStatusPartiallyRefunded
	remaining := t.Amount - t.RefundedAmount
	if remaining < 0 {
		remaining = 0
	}
	switch req.Mode {
	case RefundStrict:
		if !refundable {
			return 0, 0, false, ErrNotRefundable
		}
		if req.Requested > remaining {
			return 0, 0, false, ErrExceedsRemainder
		}
		return req.Requested, req.Requested, true, nil
	case RefundClamp:
		switch {
		case refundable:
			return req.Requested, min(req.Requested, remaining), true, nil
		case t.Status == PaymentStatusRefunded:
			return req.Requested, 0, true, nil // nothing left: record an applied 0 (D6)
		default:
			return 0, 0, false, ErrNotRefundable
		}
	default: // RefundRemainder
		if !refundable || remaining == 0 {
			return 0, 0, false, nil
		}
		return remaining, remaining, true, nil
	}
}

// refundedStatus is the status once refunded of amount has been refunded.
func refundedStatus(amount, refunded int64) PaymentStatus {
	if refunded >= amount {
		return PaymentStatusRefunded
	}
	return PaymentStatusPartiallyRefunded
}

func refundDeduction(sellerID, refundKey string, applied int64) LedgerEntry {
	return LedgerEntry{
		SellerID:    sellerID,
		Type:        LedgerTypeRefundDeduction,
		Amount:      -applied,
		Status:      LedgerStatusCompleted,
		ReferenceID: refundKey,
	}
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresSettlementLedger struct {
	pool *pgxpool.Pool
}

func NewPostgresSettlementLedger(pool *pgxpool.Pool) *PostgresSettlementLedger {
	return &PostgresSettlementLedger{pool: pool}
}

const settledTxSQL = `SELECT ` + txColumns + ` FROM payment_transactions
	WHERE order_id = $1 AND status IN (2, 4, 5)
	ORDER BY created_at DESC, id DESC LIMIT 1`

const refundColumns = `id, payment_id, source, source_id, requested_amount, amount, reason, created_at`

func scanRefund(row pgx.Row, r *Refund) error {
	return row.Scan(&r.ID, &r.PaymentID, &r.Source, &r.SourceID, &r.RequestedAmount, &r.Amount, &r.Reason, &r.CreatedAt)
}

func listRefunds(ctx context.Context, q DBTX, paymentID string) ([]Refund, error) {
	rows, err := q.Query(ctx, `SELECT `+refundColumns+` FROM payment_refunds
		WHERE payment_id = $1 ORDER BY created_at, id`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("list refunds: %w", err)
	}
	defer rows.Close()
	var out []Refund
	for rows.Next() {
		var r Refund
		if err := scanRefund(rows, &r); err != nil {
			return nil, fmt.Errorf("scan refund: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func refundByKey(ctx context.Context, q DBTX, key string) (Refund, bool, error) {
	var r Refund
	if err := scanRefund(q.QueryRow(ctx, `SELECT `+refundColumns+` FROM payment_refunds WHERE id = $1`, key), &r); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, false, nil
		}
		return Refund{}, false, fmt.Errorf("load refund: %w", err)
	}
	return r, true, nil
}

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

func (s *PostgresSettlementLedger) ListRefunds(ctx context.Context, paymentID string) ([]Refund, error) {
	return listRefunds(ctx, s.pool, paymentID)
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
	// Refunds that landed before the credit: the credit path books their deductions, for
	// the seller the credit belongs to (one per refund, design D3).
	refunds, err := listRefunds(ctx, tx, t.ID)
	if err != nil {
		return CreditResult{}, err
	}
	lockedOwner := credit.SellerID == sellerID
	for _, r := range refunds {
		if r.Amount <= 0 {
			continue
		}
		if !lockedOwner {
			if err := lockSeller(ctx, tx, credit.SellerID); err != nil {
				return CreditResult{}, err
			}
			lockedOwner = true
		}
		_, deducted, err := insertLedgerOnce(ctx, tx, refundDeduction(credit.SellerID, r.ID, r.Amount))
		if err != nil {
			return CreditResult{}, err
		}
		res.Deducted = res.Deducted || deducted
	}
	if err := tx.Commit(ctx); err != nil {
		return CreditResult{}, fmt.Errorf("commit credit: %w", err)
	}
	return res, nil
}

func (s *PostgresSettlementLedger) ApplyRefund(ctx context.Context, req RefundRequest) (RefundResult, error) {
	if err := validateRefundRequest(req); err != nil {
		return RefundResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RefundResult{}, fmt.Errorf("begin refund tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. The payment row lock serialises every refund path of this payment.
	var t PaymentTransaction
	if err := scanTransaction(tx.QueryRow(ctx, `SELECT `+txColumns+` FROM payment_transactions WHERE id = $1 FOR UPDATE`, req.PaymentID), &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefundResult{}, ErrTransactionNotFound
		}
		return RefundResult{}, fmt.Errorf("lock payment tx: %w", err)
	}
	// 2. An existing key is a replay or a conflict.
	if existing, ok, err := refundByKey(ctx, tx, req.Key); err != nil {
		return RefundResult{}, err
	} else if ok {
		if err := replayOf(existing, req); err != nil {
			return RefundResult{}, err
		}
		return RefundResult{Transaction: t, Refund: existing}, nil
	}
	// 3-4. Status and mode.
	requested, applied, write, err := plan(t, req)
	if err != nil {
		return RefundResult{}, err
	}
	if !write {
		return RefundResult{Transaction: t}, nil
	}
	// 5. The refund row. A concurrent call with the same key on another payment holds
	// another row lock: ON CONFLICT waits for it and the loser reads the winner's row.
	r := Refund{ID: req.Key, PaymentID: t.ID, Source: req.Source, SourceID: req.SourceID,
		RequestedAmount: requested, Amount: applied, Reason: req.Reason}
	err = tx.QueryRow(ctx, `INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (id) DO NOTHING RETURNING created_at`,
		r.ID, r.PaymentID, r.Source, r.SourceID, r.RequestedAmount, r.Amount, r.Reason).Scan(&r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, ok, lerr := refundByKey(ctx, tx, req.Key)
		if lerr != nil {
			return RefundResult{}, lerr
		}
		if ok && replayOf(existing, req) == nil {
			return RefundResult{Transaction: t, Refund: existing}, nil
		}
		return RefundResult{}, ErrRefundIDConflict
	}
	if err != nil {
		return RefundResult{}, fmt.Errorf("insert refund: %w", err)
	}
	res := RefundResult{Transaction: t, Refund: r, Created: true}
	if applied > 0 {
		refunded := t.RefundedAmount + applied
		const upd = `UPDATE payment_transactions
			SET refunded_amount = $2, status = $3, provider_reference = $4, updated_at = NOW()
			WHERE id = $1
			RETURNING ` + txColumns
		if err := scanTransaction(tx.QueryRow(ctx, upd, t.ID, refunded, int32(refundedStatus(t.Amount, refunded)), "REFUND:"+req.Reason), &res.Transaction); err != nil {
			return RefundResult{}, fmt.Errorf("refund payment tx: %w", err)
		}
		// 6. The deduction for a credited payment, keyed by the refund.
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
			_, deducted, err := insertLedgerOnce(ctx, tx, refundDeduction(credit.SellerID, r.ID, applied))
			if err != nil {
				return RefundResult{}, err
			}
			res.Deducted = deducted
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RefundResult{}, fmt.Errorf("commit refund: %w", err)
	}
	return res, nil
}

// ── InMemory Implementation ───────────────────────────────────────────

// refundIndex is the in-memory payment_refunds table. Its mutex is a leaf lock: the
// ledger's hold-back reads it while holding the ledger lock.
type refundIndex struct {
	mu    sync.RWMutex
	byKey map[string]Refund
	order []string // insertion order = created_at order
}

func newRefundIndex() *refundIndex { return &refundIndex{byKey: map[string]Refund{}} }

func (ix *refundIndex) get(key string) (Refund, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	r, ok := ix.byKey[key]
	return r, ok
}

func (ix *refundIndex) put(r Refund) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.byKey[r.ID] = r
	ix.order = append(ix.order, r.ID)
}

func (ix *refundIndex) list(paymentID string) []Refund {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []Refund
	for _, k := range ix.order {
		if r := ix.byKey[k]; r.PaymentID == paymentID {
			out = append(out, r)
		}
	}
	return out
}

// paymentOf maps a refund key to its payment (the hold-back join of design D3).
func (ix *refundIndex) paymentOf(key string) (string, bool) {
	r, ok := ix.get(key)
	return r.PaymentID, ok
}

// InMemorySettlementLedger is the in-memory SettlementLedger over the in-memory payment
// and ledger stores. One mutex plays the payment-row lock; the ledger's own mutex plays
// the seller lock (lock order: s.mu, payments.mu, ledger.mu, refunds.mu).
type InMemorySettlementLedger struct {
	mu       sync.Mutex
	payments *InMemoryPaymentRepository
	ledger   *InMemoryLedgerRepository
	refunds  *refundIndex
}

func NewInMemorySettlementLedger(payments *InMemoryPaymentRepository, ledger *InMemoryLedgerRepository) *InMemorySettlementLedger {
	ix := newRefundIndex()
	ledger.mu.Lock()
	ledger.refunds = ix
	ledger.mu.Unlock()
	return &InMemorySettlementLedger{payments: payments, ledger: ledger, refunds: ix}
}

func settledStatus(st PaymentStatus) bool {
	return st == PaymentStatusPaid || st == PaymentStatusPartiallyRefunded || st == PaymentStatusRefunded
}

func (s *InMemorySettlementLedger) settledLocked(orderID string) (PaymentTransaction, bool) {
	s.payments.mu.RLock()
	defer s.payments.mu.RUnlock()
	var best PaymentTransaction
	found := false
	for _, t := range s.payments.data {
		if t.OrderID != orderID || !settledStatus(t.Status) {
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

func (s *InMemorySettlementLedger) ListRefunds(_ context.Context, paymentID string) ([]Refund, error) {
	return s.refunds.list(paymentID), nil
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
	for _, r := range s.refunds.list(t.ID) {
		if r.Amount <= 0 {
			continue
		}
		_, deducted, err := s.ledger.appendOnceLocked(refundDeduction(credit.SellerID, r.ID, r.Amount))
		if err != nil {
			return CreditResult{}, err
		}
		res.Deducted = res.Deducted || deducted
	}
	return res, nil
}

func (s *InMemorySettlementLedger) ApplyRefund(_ context.Context, req RefundRequest) (RefundResult, error) {
	if err := validateRefundRequest(req); err != nil {
		return RefundResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.payments.mu.RLock()
	t, ok := s.payments.data[req.PaymentID]
	s.payments.mu.RUnlock()
	if !ok {
		return RefundResult{}, ErrTransactionNotFound
	}
	if existing, ok := s.refunds.get(req.Key); ok {
		if err := replayOf(existing, req); err != nil {
			return RefundResult{}, err
		}
		return RefundResult{Transaction: t, Refund: existing}, nil
	}
	requested, applied, write, err := plan(t, req)
	if err != nil {
		return RefundResult{}, err
	}
	if !write {
		return RefundResult{Transaction: t}, nil
	}
	r := Refund{ID: req.Key, PaymentID: t.ID, Source: req.Source, SourceID: req.SourceID,
		RequestedAmount: requested, Amount: applied, Reason: req.Reason, CreatedAt: time.Now()}
	s.refunds.put(r)
	res := RefundResult{Transaction: t, Refund: r, Created: true}
	if applied == 0 {
		return res, nil
	}
	t.RefundedAmount += applied
	t.Status = refundedStatus(t.Amount, t.RefundedAmount)
	t.ProviderReference = "REFUND:" + req.Reason
	t.UpdatedAt = time.Now()
	s.payments.mu.Lock()
	s.payments.data[t.ID] = t
	s.payments.mu.Unlock()
	res.Transaction = t

	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	if credit, ok := s.ledger.byReferenceLocked(LedgerTypeOrderSettlement, t.ID); ok {
		_, deducted, err := s.ledger.appendOnceLocked(refundDeduction(credit.SellerID, r.ID, applied))
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
