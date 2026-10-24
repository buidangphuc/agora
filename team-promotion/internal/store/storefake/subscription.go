package storefake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// InMemorySubscriptionRepository is the DB-less backend used when
// DATABASE_ENABLED=false and in unit tests. Plans are seeded from DefaultPlans.
type InMemorySubscriptionRepository struct {
	mu    sync.RWMutex
	plans map[string]store.Plan               // keyed by id
	subs  map[string]store.SellerSubscription // keyed by seller_id (one per seller)
}

func NewInMemorySubscriptionRepository() *InMemorySubscriptionRepository {
	plans := make(map[string]store.Plan)
	for _, p := range store.DefaultPlans() {
		plans[p.ID] = p
	}
	return &InMemorySubscriptionRepository{
		plans: plans,
		subs:  make(map[string]store.SellerSubscription),
	}
}

func (r *InMemorySubscriptionRepository) ListPlans(_ context.Context) ([]store.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]store.Plan, 0, len(r.plans))
	for _, p := range r.plans {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tier < out[j].Tier })
	return out, nil
}

func (r *InMemorySubscriptionRepository) GetPlanByID(_ context.Context, id string) (store.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plans[id]
	if !ok {
		return store.Plan{}, store.ErrPlanNotFound
	}
	return p, nil
}

func (r *InMemorySubscriptionRepository) UpsertSubscription(_ context.Context, sub store.SellerSubscription) (store.SellerSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sub.Status == "" {
		sub.Status = store.SubscriptionStatusActive
	}
	if existing, ok := r.subs[sub.SellerID]; ok {
		// Replace plan/status, keep original id + created_at.
		existing.PlanID = sub.PlanID
		existing.Status = sub.Status
		existing.Tier = sub.Tier
		r.subs[sub.SellerID] = existing
		return existing, nil
	}
	if sub.ID == "" {
		sub.ID = store.NewID()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	r.subs[sub.SellerID] = sub
	return sub, nil
}

func (r *InMemorySubscriptionRepository) GetActiveSubscriptionBySeller(_ context.Context, sellerID string) (store.SellerSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub, ok := r.subs[sellerID]
	if !ok || sub.Status != store.SubscriptionStatusActive {
		return store.SellerSubscription{}, store.ErrSubscriptionNotFound
	}
	// Denormalize the current plan tier in (mirrors the Postgres join).
	if p, ok := r.plans[sub.PlanID]; ok {
		sub.Tier = p.Tier
	}
	return sub, nil
}

func (r *InMemorySubscriptionRepository) Ping(_ context.Context) error { return nil }
