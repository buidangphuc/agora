package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/buidangphuc/team-payment/internal/store"
)

// refundIndex is the in-memory payment_refunds table. Its mutex is a leaf lock: the
// ledger's hold-back reads it while holding the ledger lock.
type refundIndex struct {
	mu    sync.RWMutex
	byKey map[string]store.Refund
	order []string // insertion order = created_at order
}

func newRefundIndex() *refundIndex { return &refundIndex{byKey: map[string]store.Refund{}} }

func (ix *refundIndex) get(key string) (store.Refund, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	r, ok := ix.byKey[key]
	return r, ok
}

func (ix *refundIndex) put(r store.Refund) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.byKey[r.ID] = r
	ix.order = append(ix.order, r.ID)
}

func (ix *refundIndex) list(paymentID string) []store.Refund {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []store.Refund
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

// InMemorySettlementLedger is the in-memory store.SettlementLedger over the in-memory payment
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

func settledStatus(st store.PaymentStatus) bool {
	return st == store.PaymentStatusPaid || st == store.PaymentStatusPartiallyRefunded || st == store.PaymentStatusRefunded
}

func (s *InMemorySettlementLedger) settledLocked(orderID string) (store.PaymentTransaction, bool) {
	s.payments.mu.RLock()
	defer s.payments.mu.RUnlock()
	var best store.PaymentTransaction
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

func (s *InMemorySettlementLedger) SettledTransaction(_ context.Context, orderID string) (store.PaymentTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.settledLocked(orderID)
	if !ok {
		return store.PaymentTransaction{}, store.ErrNotSettled
	}
	return t, nil
}

func (s *InMemorySettlementLedger) ListRefunds(_ context.Context, paymentID string) ([]store.Refund, error) {
	return s.refunds.list(paymentID), nil
}

func (s *InMemorySettlementLedger) CreditSettlement(_ context.Context, orderID, sellerID string) (store.CreditResult, error) {
	if sellerID == "" {
		return store.CreditResult{}, errors.New("seller id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.settledLocked(orderID)
	if !ok {
		return store.CreditResult{}, store.ErrNotSettled
	}
	if t.Amount <= 0 {
		return store.CreditResult{}, store.ErrInvalidAmount
	}
	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	credit, credited, err := s.ledger.appendOnceLocked(store.LedgerEntry{
		SellerID: sellerID, Type: store.LedgerTypeOrderSettlement, Amount: t.Amount,
		Status: store.LedgerStatusCompleted, ReferenceID: t.ID,
	})
	if err != nil {
		return store.CreditResult{}, err
	}
	res := store.CreditResult{Transaction: t, Credit: credit, Credited: credited}
	for _, r := range s.refunds.list(t.ID) {
		if r.Amount <= 0 {
			continue
		}
		_, deducted, err := s.ledger.appendOnceLocked(store.RefundDeduction(credit.SellerID, r.ID, r.Amount))
		if err != nil {
			return store.CreditResult{}, err
		}
		res.Deducted = res.Deducted || deducted
	}
	return res, nil
}

func (s *InMemorySettlementLedger) ApplyRefund(_ context.Context, req store.RefundRequest) (store.RefundResult, error) {
	if err := store.ValidateRefundRequest(req); err != nil {
		return store.RefundResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.payments.mu.RLock()
	t, ok := s.payments.data[req.PaymentID]
	s.payments.mu.RUnlock()
	if !ok {
		return store.RefundResult{}, store.ErrTransactionNotFound
	}
	if existing, ok := s.refunds.get(req.Key); ok {
		if err := store.ReplayOf(existing, req); err != nil {
			return store.RefundResult{}, err
		}
		return store.RefundResult{Transaction: t, Refund: existing}, nil
	}
	requested, applied, write, err := store.Plan(t, req)
	if err != nil {
		return store.RefundResult{}, err
	}
	if !write {
		return store.RefundResult{Transaction: t}, nil
	}
	r := store.Refund{ID: req.Key, PaymentID: t.ID, Source: req.Source, SourceID: req.SourceID,
		RequestedAmount: requested, Amount: applied, Reason: req.Reason, CreatedAt: time.Now()}
	s.refunds.put(r)
	res := store.RefundResult{Transaction: t, Refund: r, Created: true}
	if applied == 0 {
		return res, nil
	}
	t.RefundedAmount += applied
	t.Status = store.RefundedStatus(t.Amount, t.RefundedAmount)
	t.ProviderReference = "REFUND:" + req.Reason
	t.UpdatedAt = time.Now()
	s.payments.mu.Lock()
	s.payments.data[t.ID] = t
	s.payments.mu.Unlock()
	res.Transaction = t

	s.ledger.mu.Lock()
	defer s.ledger.mu.Unlock()
	if credit, ok := s.ledger.byReferenceLocked(store.LedgerTypeOrderSettlement, t.ID); ok {
		_, deducted, err := s.ledger.appendOnceLocked(store.RefundDeduction(credit.SellerID, r.ID, applied))
		if err != nil {
			return store.RefundResult{}, err
		}
		res.Deducted = deducted
	}
	return res, nil
}

var (
	_ store.SettlementLedger = (*InMemorySettlementLedger)(nil)
)
