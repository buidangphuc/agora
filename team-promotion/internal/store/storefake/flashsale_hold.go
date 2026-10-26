package storefake

import (
	"context"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemoryFlashHoldRepository shares the campaign repository's mutex, so a hold
// and the stock_sold counter always change together.
type InMemoryFlashHoldRepository struct {
	camp  *InMemoryFlashSaleRepository
	holds map[string]store.FlashHold
}

func NewInMemoryFlashHoldRepository(c *InMemoryFlashSaleRepository) *InMemoryFlashHoldRepository {
	return &InMemoryFlashHoldRepository{camp: c, holds: make(map[string]store.FlashHold)}
}

func (r *InMemoryFlashHoldRepository) heldLocked(campaignID string, now time.Time) int64 {
	var n int64
	for _, h := range r.holds {
		if h.CampaignID == campaignID && h.Status == store.FlashHoldReserved && now.Before(h.ExpiresAt) {
			n += h.Quantity
		}
	}
	return n
}

func (r *InMemoryFlashHoldRepository) Reserve(_ context.Context, h store.FlashHold, now time.Time) (store.FlashHold, bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	c, ok := r.camp.campaigns[h.CampaignID]
	if !ok {
		return store.FlashHold{}, false, store.ErrCampaignNotFound
	}
	if stored, ok := r.holds[h.ReservationID]; ok {
		return stored, false, nil
	}
	if h.Quantity > c.StockCap-c.StockSold-r.heldLocked(c.ID, now) {
		return store.FlashHold{}, false, store.ErrFlashStockInsufficient
	}
	h.Status = store.FlashHoldReserved
	h.ExpiresAt = now.Add(store.FlashHoldTTL)
	r.holds[h.ReservationID] = h
	return h, true, nil
}

func (r *InMemoryFlashHoldRepository) Get(_ context.Context, id string) (store.FlashHold, error) {
	r.camp.mu.RLock()
	defer r.camp.mu.RUnlock()
	h, ok := r.holds[id]
	if !ok {
		return store.FlashHold{}, store.ErrFlashHoldNotFound
	}
	return h, nil
}

func (r *InMemoryFlashHoldRepository) Commit(_ context.Context, id string, now time.Time) (bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	h, ok := r.holds[id]
	if !ok {
		return false, nil
	}
	switch h.Status {
	case store.FlashHoldCommitted:
		return true, nil
	case store.FlashHoldReleased:
		return false, nil
	}
	if !now.Before(h.ExpiresAt) {
		return false, nil
	}
	c := r.camp.campaigns[h.CampaignID]
	c.StockSold += h.Quantity
	r.camp.campaigns[h.CampaignID] = c
	h.Status = store.FlashHoldCommitted
	r.holds[id] = h
	return true, nil
}

func (r *InMemoryFlashHoldRepository) Release(_ context.Context, id string, now time.Time) (bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	h, ok := r.holds[id]
	if !ok {
		return false, nil
	}
	if h.Status == store.FlashHoldReleased {
		return true, nil
	}
	if h.Status == store.FlashHoldCommitted {
		c := r.camp.campaigns[h.CampaignID]
		c.StockSold -= h.Quantity
		r.camp.campaigns[h.CampaignID] = c
	}
	h.Status = store.FlashHoldReleased
	h.ReleasedAt = now
	r.holds[id] = h
	return true, nil
}

func (r *InMemoryFlashHoldRepository) ActiveHeld(_ context.Context, campaignID string, now time.Time) (int64, error) {
	r.camp.mu.RLock()
	defer r.camp.mu.RUnlock()
	return r.heldLocked(campaignID, now), nil
}

func (r *InMemoryFlashHoldRepository) PurgeReleasedAndExpiredHolds(_ context.Context, before time.Time) (int64, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	var n int64
	for id, h := range r.holds {
		if (h.Status == store.FlashHoldReleased && h.ReleasedAt.Before(before)) ||
			(h.Status == store.FlashHoldReserved && h.ExpiresAt.Before(before)) {
			delete(r.holds, id)
			n++
		}
	}
	return n, nil
}
