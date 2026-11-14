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
	tokens    map[string]*fakeImpression
}

type fakeImpression struct {
	campaignID string
	listingID  string
	expiresAt  time.Time
	clicked    bool
}

func NewInMemoryAdCampaignRepository() *InMemoryAdCampaignRepository {
	return &InMemoryAdCampaignRepository{campaigns: make(map[string]store.AdCampaign), tokens: make(map[string]*fakeImpression)}
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

func (r *InMemoryAdCampaignRepository) ListServable(_ context.Context, category string, limit int) ([]store.AdCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.AdCampaign
	for _, c := range r.campaigns {
		if c.Status != store.AdCampaignStatusActive || c.Budget-c.Spent < c.Bid {
			continue
		}
		if category != "" && c.CategoryID != category {
			continue
		}
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Bid != all[j].Bid {
			return all[i].Bid > all[j].Bid
		}
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryAdCampaignRepository) IssueImpressions(_ context.Context, issues []store.ImpressionIssue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range issues {
		c, ok := r.campaigns[i.CampaignID]
		if !ok {
			return store.ErrAdCampaignNotFound
		}
		c.Impressions++
		r.campaigns[i.CampaignID] = c
		r.tokens[i.Token] = &fakeImpression{campaignID: i.CampaignID, listingID: i.ListingID, expiresAt: time.Now().Add(store.ImpressionTTL)}
	}
	return nil
}

func (r *InMemoryAdCampaignRepository) RecordClick(_ context.Context, token, principalID string, now time.Time) (store.ClickResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	imp, ok := r.tokens[token]
	if !ok {
		return store.ClickResult{}, store.ErrImpressionNotFound
	}
	res := store.ClickResult{ListingID: imp.listingID}
	if imp.clicked || now.After(imp.expiresAt) {
		return res, nil
	}
	c, ok := r.campaigns[imp.campaignID]
	if !ok {
		return store.ClickResult{}, store.ErrAdCampaignNotFound
	}
	if principalID != "" && principalID == c.SellerID {
		return res, nil
	}
	if c.Status != store.AdCampaignStatusActive || c.Budget-c.Spent < c.Bid {
		return res, nil
	}
	c.Spent += c.Bid
	c.Clicks++
	if c.Budget-c.Spent < c.Bid {
		c.Status = store.AdCampaignStatusPausedBudget
	}
	r.campaigns[c.ID] = c
	imp.clicked = true
	res.Charged = true
	return res, nil
}
