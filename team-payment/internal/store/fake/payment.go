package fake

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-payment/internal/store"
)

type InMemoryPaymentRepository struct {
	mu   sync.RWMutex
	data map[string]store.PaymentTransaction
}

func NewInMemoryPaymentRepository() *InMemoryPaymentRepository {
	return &InMemoryPaymentRepository{
		data: make(map[string]store.PaymentTransaction),
	}
}

func (r *InMemoryPaymentRepository) CreateTransaction(_ context.Context, t store.PaymentTransaction) (store.PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Currency == "" {
		t.Currency = "VND"
	}
	if t.Status == 0 {
		t.Status = store.PaymentStatusPending
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	r.data[t.ID] = t
	return t, nil
}

func (r *InMemoryPaymentRepository) GetTransaction(_ context.Context, id string) (store.PaymentTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.data[id]
	if !ok {
		return store.PaymentTransaction{}, store.ErrTransactionNotFound
	}
	return t, nil
}

func (r *InMemoryPaymentRepository) GetTransactionByOrderID(_ context.Context, orderID string) (store.PaymentTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.data {
		if t.OrderID == orderID {
			return t, nil
		}
	}
	return store.PaymentTransaction{}, store.ErrTransactionNotFound
}

func (r *InMemoryPaymentRepository) UpdateStatus(_ context.Context, id string, status store.PaymentStatus, providerRef string) (store.PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.data[id]
	if !ok {
		return store.PaymentTransaction{}, store.ErrTransactionNotFound
	}
	t.Status = status
	t.ProviderReference = providerRef
	t.UpdatedAt = time.Now()
	r.data[id] = t
	return t, nil
}

func (r *InMemoryPaymentRepository) UpdateTransactionStatus(_ context.Context, id string, status store.PaymentStatus, providerRef string) (store.PaymentTransaction, error) {
	return r.updateStatusFromSettleable(id, status, providerRef)
}

// updateStatusFromSettleable is the compare-and-set the Postgres writer runs:
// the write applies only to a PENDING or FAILED payment.
func (r *InMemoryPaymentRepository) updateStatusFromSettleable(id string, status store.PaymentStatus, providerRef string) (store.PaymentTransaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.data[id]
	if !ok {
		return store.PaymentTransaction{}, store.ErrTransactionNotFound
	}
	if t.Status != store.PaymentStatusPending && t.Status != store.PaymentStatusFailed {
		return store.PaymentTransaction{}, store.ErrNotSettleable
	}
	t.Status = status
	t.ProviderReference = providerRef
	t.UpdatedAt = time.Now()
	r.data[id] = t
	return t, nil
}

// CancelUnpaid is the in-memory compare-and-set PENDING|FAILED -> CANCELLED.
func (r *InMemoryPaymentRepository) CancelUnpaid(_ context.Context, orderID string) (store.CancelUnpaidOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	outcome := store.CancelAbsent
	for id, t := range r.data {
		if t.OrderID != orderID {
			continue
		}
		switch t.Status {
		case store.PaymentStatusPending, store.PaymentStatusFailed:
			t.Status = store.PaymentStatusCancelled
			t.UpdatedAt = time.Now()
			r.data[id] = t
			outcome = store.CancelVoided
		case store.PaymentStatusCancelled:
			if outcome == store.CancelAbsent {
				outcome = store.CancelAlreadyCancelled
			}
		default:
			if outcome == store.CancelAbsent {
				outcome = store.CancelOtherStatus
			}
		}
	}
	return outcome, nil
}
