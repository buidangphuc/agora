package storefake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemoryFlashSaleRepository is the DB-less backend used when DATABASE_ENABLED
// is false and in unit tests.
type InMemoryFlashSaleRepository struct {
	mu        sync.RWMutex
	campaigns map[string]store.FlashSaleCampaign
}

func NewInMemoryFlashSaleRepository() *InMemoryFlashSaleRepository {
	return &InMemoryFlashSaleRepository{campaigns: make(map[string]store.FlashSaleCampaign)}
}

// active reports whether now falls within the campaign's [starts_at, ends_at)
// window; a zero bound is treated as open-ended.
func active(c store.FlashSaleCampaign, now time.Time) bool {
	if !c.StartsAt.IsZero() && now.Before(c.StartsAt) {
		return false
	}
	if !c.EndsAt.IsZero() && !now.Before(c.EndsAt) {
		return false
	}
	return true
}

func (r *InMemoryFlashSaleRepository) Create(_ context.Context, c store.FlashSaleCampaign, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = store.NewID()
	}
	if _, err := event(c); err != nil {
		return store.FlashSaleCampaign{}, err
	}
	r.campaigns[c.ID] = c
	return c, nil
}

func (r *InMemoryFlashSaleRepository) GetByID(_ context.Context, id string) (store.FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.campaigns[id]
	if !ok {
		return store.FlashSaleCampaign{}, store.ErrCampaignNotFound
	}
	return c, nil
}

func (r *InMemoryFlashSaleRepository) GetActiveByListing(_ context.Context, listingID string, now time.Time) (store.FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found []store.FlashSaleCampaign
	for _, c := range r.campaigns {
		if c.ListingID == listingID && active(c, now) {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		return store.FlashSaleCampaign{}, store.ErrCampaignNotFound
	}
	// Prefer the most recently started campaign, mirroring the Postgres ordering.
	sort.Slice(found, func(i, j int) bool { return found[i].StartsAt.After(found[j].StartsAt) })
	return found[0], nil
}

func (r *InMemoryFlashSaleRepository) ListActive(_ context.Context, now time.Time, limit, offset int) ([]store.FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.FlashSaleCampaign
	for _, c := range r.campaigns {
		if active(c, now) {
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].StartsAt.After(all[j].StartsAt) })
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryFlashSaleRepository) Ping(_ context.Context) error { return nil }
