package fake

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-payment/internal/store"
)

type InMemoryWalletRepository struct {
	mu      sync.RWMutex
	payouts map[string]store.PayoutRequest // keyed by id
}

func NewInMemoryWalletRepository() *InMemoryWalletRepository {
	return &InMemoryWalletRepository{payouts: make(map[string]store.PayoutRequest)}
}

func (r *InMemoryWalletRepository) CreatePayoutRequest(_ context.Context, p store.PayoutRequest) (store.PayoutRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Status == 0 {
		p.Status = store.PayoutStatusPending
	}
	p.CreatedAt = time.Now()
	r.payouts[p.ID] = p
	return p, nil
}

func (r *InMemoryWalletRepository) GetPayoutRequest(_ context.Context, id string) (store.PayoutRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.payouts[id]
	if !ok {
		return store.PayoutRequest{}, store.ErrPayoutNotFound
	}
	return p, nil
}

func (r *InMemoryWalletRepository) ListPayoutRequestsBySellerID(_ context.Context, sellerID string) ([]store.PayoutRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []store.PayoutRequest
	for _, p := range r.payouts {
		if p.SellerID == sellerID {
			result = append(result, p)
		}
	}
	// Sort by CreatedAt desc
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].CreatedAt.Before(result[j].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}
