package storefake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

type fakeSubscriptionTx struct {
	r      *InMemorySubscriptionRepository
	seller string
	row    store.SellerSubscription
	loaded bool
}

func (t *fakeSubscriptionTx) Get(_ context.Context) (store.SellerSubscription, error) {
	if !t.loaded {
		t.r.mu.RLock()
		row, ok := t.r.subs[t.seller]
		t.r.mu.RUnlock()
		if !ok {
			row = store.SellerSubscription{
				ID: store.NewID(), SellerID: t.seller, PlanID: store.FreePlan().ID,
				Status: store.SubscriptionStatusExpired, CreatedAt: time.Now(),
			}
		}
		t.row, t.loaded = row, true
	}
	return t.withTier(t.row), nil
}

func (t *fakeSubscriptionTx) withTier(s store.SellerSubscription) store.SellerSubscription {
	if p, ok := t.r.plans[s.PlanID]; ok {
		s.Tier = p.Tier
	}
	return s
}

func (t *fakeSubscriptionTx) UpdatePeriod(ctx context.Context, p store.SubscriptionPeriod) (store.SellerSubscription, error) {
	if _, err := t.Get(ctx); err != nil {
		return store.SellerSubscription{}, err
	}
	t.row.PlanID = p.PlanID
	t.row.Status = p.Status
	t.row.CurrentPeriodStart = p.CurrentPeriodStart
	t.row.CurrentPeriodEnd = p.CurrentPeriodEnd
	t.row.AutoRenew = p.AutoRenew
	t.row.PendingPlanID = p.PendingPlanID
	t.row.PeriodSeq = p.PeriodSeq
	return t.withTier(t.row), nil
}

var sellerLocks sync.Map

func (r *InMemorySubscriptionRepository) sellerLock(sellerID string) *sync.Mutex {
	key := struct {
		r *InMemorySubscriptionRepository
		s string
	}{r, sellerID}
	m, _ := sellerLocks.LoadOrStore(key, &sync.Mutex{})
	return m.(*sync.Mutex)
}

func (r *InMemorySubscriptionRepository) WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx store.SubscriptionTx) error) error {
	lock := r.sellerLock(sellerID)
	lock.Lock()
	defer lock.Unlock()
	tx := &fakeSubscriptionTx{r: r, seller: sellerID}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if tx.loaded {
		r.mu.Lock()
		r.subs[sellerID] = tx.row
		r.mu.Unlock()
	}
	return nil
}

func (r *InMemorySubscriptionRepository) ListDueForRenewal(_ context.Context, now time.Time, limit int) ([]store.SellerSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []store.SellerSubscription
	for _, s := range r.subs {
		if s.Status == store.SubscriptionStatusActive && !s.CurrentPeriodEnd.IsZero() && !s.CurrentPeriodEnd.After(now) {
			if p, ok := r.plans[s.PlanID]; ok {
				s.Tier = p.Tier
			}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CurrentPeriodEnd.Equal(out[j].CurrentPeriodEnd) {
			return out[i].CurrentPeriodEnd.Before(out[j].CurrentPeriodEnd)
		}
		return out[i].ID < out[j].ID
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
