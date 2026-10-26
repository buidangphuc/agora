package storefake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemoryAdCampaignRepository is the DB-less backend used when
// DATABASE_ENABLED=false and in unit tests.
type InMemoryAdCampaignRepository struct {
	mu        sync.RWMutex
	campaigns map[string]store.AdCampaign // keyed by id
}

func NewInMemoryAdCampaignRepository() *InMemoryAdCampaignRepository {
	return &InMemoryAdCampaignRepository{campaigns: make(map[string]store.AdCampaign)}
}

func (r *InMemoryAdCampaignRepository) Create(_ context.Context, c store.AdCampaign) (store.AdCampaign, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = store.NewID()
	}
	if c.Status == 0 {
		c.Status = store.AdCampaignStatusActive
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	r.campaigns[c.ID] = c
	return c, nil
}

func (r *InMemoryAdCampaignRepository) GetByID(_ context.Context, id string) (store.AdCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.campaigns[id]
	if !ok {
		return store.AdCampaign{}, store.ErrAdCampaignNotFound
	}
	return c, nil
}

func (r *InMemoryAdCampaignRepository) ListBySeller(_ context.Context, sellerID string, limit, offset int) ([]store.AdCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.AdCampaign
	for _, c := range r.campaigns {
		if c.SellerID == sellerID {
			all = append(all, c)
		}
	}
	// Newest first (mirrors the Postgres ORDER BY created_at DESC); ties break on
	// id for a deterministic order in the DB-less path.
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryAdCampaignRepository) ListActive(_ context.Context, limit int) ([]store.AdCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.AdCampaign
	for _, c := range r.campaigns {
		if c.Status == store.AdCampaignStatusActive {
			all = append(all, c)
		}
	}
	// Best bid first; ties break on newest-first then id for a stable order
	// (mirrors the Postgres ORDER BY bid DESC, created_at DESC).
	sort.Slice(all, func(i, j int) bool {
		if all[i].Bid != all[j].Bid {
			return all[i].Bid > all[j].Bid
		}
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryAdCampaignRepository) Ping(_ context.Context) error { return nil }
