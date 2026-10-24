package storetest

import (
	"context"
	"github.com/buidangphuc/team-domain/internal/store"
	"sync"
	"time"
)

type InMemoryStorefrontRepository struct {
	mu       sync.RWMutex
	bySeller map[string]store.Storefront
}

func NewInMemoryStorefrontRepository(seed ...store.Storefront) *InMemoryStorefrontRepository {
	bySeller := make(map[string]store.Storefront, len(seed))
	for _, s := range seed {
		bySeller[s.SellerID] = s
	}
	return &InMemoryStorefrontRepository{bySeller: bySeller}
}
func (r *InMemoryStorefrontRepository) Upsert(_ context.Context, s store.Storefront) (store.Storefront, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for sellerID, existing := range r.bySeller {
		if existing.Slug == s.Slug && sellerID != s.SellerID {
			return store.Storefront{}, store.ErrSlugTaken
		}
	}
	s.UpdatedAt = time.Now().UTC()
	r.bySeller[s.SellerID] = s
	return s, nil
}
func (r *InMemoryStorefrontRepository) GetBySeller(_ context.Context, sellerID string) (store.Storefront, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.bySeller[sellerID]
	if !ok {
		return store.Storefront{}, store.ErrStorefrontNotFound
	}
	return s, nil
}
func (r *InMemoryStorefrontRepository) GetBySlug(_ context.Context, slug string) (store.Storefront, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.bySeller {
		if s.Slug == slug {
			return s, nil
		}
	}
	return store.Storefront{}, store.ErrStorefrontNotFound
}
func (r *InMemoryStorefrontRepository) GetBySellers(_ context.Context, sellerIDs []string) ([]store.Storefront, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]store.Storefront, 0, len(sellerIDs))
	for _, id := range sellerIDs {
		if s, ok := r.bySeller[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

var _ store.StorefrontRepository = (*InMemoryStorefrontRepository)(nil)
