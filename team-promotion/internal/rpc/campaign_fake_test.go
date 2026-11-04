package rpc_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type campaignFake struct {
	*storefake.InMemoryFlashSaleRepository

	mu        sync.Mutex
	ids       []string
	cancelled map[string]time.Time
	edits     map[string]store.FlashSaleCampaign
	idem      map[string]store.IdempotentCampaign
}

func newCampaignFake(inner *storefake.InMemoryFlashSaleRepository) *campaignFake {
	return &campaignFake{
		InMemoryFlashSaleRepository: inner,
		cancelled:                   map[string]time.Time{},
		edits:                       map[string]store.FlashSaleCampaign{},
		idem:                        map[string]store.IdempotentCampaign{},
	}
}

func (f *campaignFake) Create(ctx context.Context, c store.FlashSaleCampaign, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, error) {
	out, err := f.InMemoryFlashSaleRepository.Create(ctx, c, event)
	if err != nil {
		return out, err
	}
	f.mu.Lock()
	f.ids = append(f.ids, out.ID)
	f.mu.Unlock()
	return out, nil
}

func (f *campaignFake) GetByID(ctx context.Context, id string) (store.FlashSaleCampaign, error) {
	c, err := f.InMemoryFlashSaleRepository.GetByID(ctx, id)
	if err != nil {
		return c, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.apply(c), nil
}

func (f *campaignFake) apply(c store.FlashSaleCampaign) store.FlashSaleCampaign {
	if e, ok := f.edits[c.ID]; ok {
		c.SalePrice, c.StockCap, c.StartsAt, c.EndsAt = e.SalePrice, e.StockCap, e.StartsAt, e.EndsAt
	}
	c.CancelledAt = f.cancelled[c.ID]
	return c
}

func (f *campaignFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ids)
}

func (f *campaignFake) statusLocked(c store.FlashSaleCampaign, now time.Time) string {
	switch {
	case !f.cancelled[c.ID].IsZero():
		return store.CampaignStatusCancelled
	case !c.StartsAt.IsZero() && c.StartsAt.After(now):
		return store.CampaignStatusScheduled
	case !c.EndsAt.IsZero() && !c.EndsAt.After(now):
		return store.CampaignStatusEnded
	default:
		return store.CampaignStatusActive
	}
}

func (f *campaignFake) ListCampaigns(ctx context.Context, statuses []string, now time.Time, offset, limit int) ([]store.FlashSaleCampaign, error) {
	var all []store.FlashSaleCampaign
	f.mu.Lock()
	ids := append([]string(nil), f.ids...)
	f.mu.Unlock()
	for _, id := range ids {
		c, err := f.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		f.mu.Lock()
		st := f.statusLocked(c, now)
		f.mu.Unlock()
		if len(statuses) > 0 && !hasStatus(statuses, st) {
			continue
		}
		all = append(all, c)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].StartsAt.Equal(all[j].StartsAt) {
			return all[i].StartsAt.After(all[j].StartsAt)
		}
		return all[i].ID < all[j].ID
	})
	if offset > len(all) {
		offset = len(all)
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func hasStatus(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (f *campaignFake) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	c, err := f.InMemoryFlashSaleRepository.GetByID(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if at, ok := f.cancelled[id]; ok {
		return at, nil
	}
	c = f.apply(c)
	if !c.EndsAt.IsZero() && !c.EndsAt.After(now) {
		return time.Time{}, store.ErrCampaignEnded
	}
	f.cancelled[id] = now
	return now, nil
}

func (f *campaignFake) cancelledAt(id string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.cancelled[id]
	return at, ok
}

func (f *campaignFake) UpdateIfScheduled(ctx context.Context, id string, next store.FlashSaleCampaign, now time.Time) (store.FlashSaleCampaign, error) {
	c, err := f.InMemoryFlashSaleRepository.GetByID(ctx, id)
	if err != nil {
		return store.FlashSaleCampaign{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cancelled[id].IsZero() {
		return store.FlashSaleCampaign{}, store.ErrCampaignCancelled
	}
	c = f.apply(c)
	if !c.StartsAt.After(now) {
		return store.FlashSaleCampaign{}, store.ErrCampaignNotScheduled
	}
	f.edits[id] = next
	return f.apply(c), nil
}

func (f *campaignFake) LookupIdempotent(_ context.Context, actorID, key string, _ time.Time) (store.IdempotentCampaign, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if hit, ok := f.idem[actorID+"/"+key]; ok {
		return hit, nil
	}
	return store.IdempotentCampaign{}, store.ErrIdempotencyNotFound
}

func (f *campaignFake) CreateIdempotent(ctx context.Context, actorID, key, payloadHash string, c store.FlashSaleCampaign, _ time.Time, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, bool, error) {
	out, err := f.Create(ctx, c, event)
	if err != nil {
		return out, false, err
	}
	f.mu.Lock()
	f.idem[actorID+"/"+key] = store.IdempotentCampaign{Campaign: out, PayloadHash: payloadHash}
	f.mu.Unlock()
	return out, true, nil
}
