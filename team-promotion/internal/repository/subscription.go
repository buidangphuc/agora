package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPlanNotFound is returned when a plan lookup misses.
var ErrPlanNotFound = errors.New("plan not found")

// ErrSubscriptionNotFound is returned when a seller has no (active) subscription.
var ErrSubscriptionNotFound = errors.New("subscription not found")

// Subscription tier enum values (mirror platform.promotion.v1.PlanTier without
// importing the wire enum into the persistence layer).
const (
	PlanTierFree    int32 = 1
	PlanTierPro     int32 = 2
	PlanTierPremium int32 = 3
)

// Subscription status values, stored as text in seller_subscriptions.status and
// mapped to platform.promotion.v1.SubscriptionStatus in the service layer.
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusCancelled = "cancelled"
)

// Plan is the persistence-layer view of a subscription plan (reference data).
// Field semantics mirror platform.promotion.v1.Plan plus the entitlement limits
// surfaced by GetEntitlements.
type Plan struct {
	ID       string
	Tier     int32
	Price    int64             // minor units per billing period
	Features []string          // human-readable bullets (Vietnamese copy)
	Limits   map[string]string // entitlement limits keyed by feature
}

// SellerSubscription is the persistence-layer view of a seller's subscription.
// Tier is denormalized from the plan on read (join) for convenience; the table
// itself stores only seller_id, plan_id, status, created_at.
type SellerSubscription struct {
	ID        string
	SellerID  string
	PlanID    string
	Tier      int32
	Status    string
	CreatedAt time.Time
}

// DefaultPlans is the canonical FREE/PRO/PREMIUM catalog. It is the single source
// of truth shared by the in-memory repository (seeded in its constructor) and the
// SQL seed in migration 0002 — keep the two in sync. Money stays mock: prices are
// display-only minor units, this service never charges (AGENTS.md §7).
func DefaultPlans() []Plan {
	return []Plan{
		{
			ID:    "plan_free",
			Tier:  PlanTierFree,
			Price: 0,
			Features: []string{
				"Đăng tối đa 10 tin",
				"Thống kê cơ bản",
			},
			Limits: map[string]string{
				"max_listings":      "10",
				"featured_listings": "0",
				"analytics":         "basic",
				"priority_support":  "false",
			},
		},
		{
			ID:    "plan_pro",
			Tier:  PlanTierPro,
			Price: 19900000, // 199.000đ (minor units) — display only, never charged
			Features: []string{
				"Đăng tối đa 100 tin",
				"5 tin nổi bật mỗi tháng",
				"Thống kê nâng cao",
			},
			Limits: map[string]string{
				"max_listings":      "100",
				"featured_listings": "5",
				"analytics":         "advanced",
				"priority_support":  "false",
			},
		},
		{
			ID:    "plan_premium",
			Tier:  PlanTierPremium,
			Price: 49900000, // 499.000đ (minor units) — display only, never charged
			Features: []string{
				"Đăng tin không giới hạn",
				"50 tin nổi bật mỗi tháng",
				"Thống kê nâng cao",
				"Hỗ trợ ưu tiên",
			},
			Limits: map[string]string{
				"max_listings":      "-1", // unlimited
				"featured_listings": "50",
				"analytics":         "advanced",
				"priority_support":  "true",
			},
		},
	}
}

// FreePlan returns the default FREE plan — the fallback entitlement for a seller
// with no active subscription.
func FreePlan() Plan {
	for _, p := range DefaultPlans() {
		if p.Tier == PlanTierFree {
			return p
		}
	}
	return Plan{ID: "plan_free", Tier: PlanTierFree, Limits: map[string]string{}}
}

// SubscriptionRepository is the persistence seam for subscription plans and
// seller subscriptions.
type SubscriptionRepository interface {
	// ListPlans returns the plan catalog ordered by tier (FREE, PRO, PREMIUM).
	ListPlans(ctx context.Context) ([]Plan, error)
	// GetPlanByID looks a plan up by id (ErrPlanNotFound on miss).
	GetPlanByID(ctx context.Context, id string) (Plan, error)
	// UpsertSubscription records a seller's subscription. A seller has at most one
	// row: subscribing again replaces the existing plan (idempotent per seller).
	UpsertSubscription(ctx context.Context, sub SellerSubscription) (SellerSubscription, error)
	// GetActiveSubscriptionBySeller returns a seller's active subscription with the
	// plan tier denormalized in (ErrSubscriptionNotFound when the seller has none).
	GetActiveSubscriptionBySeller(ctx context.Context, sellerID string) (SellerSubscription, error)
	// Ping verifies the backing store is reachable.
	Ping(ctx context.Context) error
}

// ── Postgres implementation ──

// PostgresSubscriptionRepository persists subscriptions in promotion_db. Plans are
// read-only reference data seeded by migration 0002.
type PostgresSubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSubscriptionRepository(pool *pgxpool.Pool) *PostgresSubscriptionRepository {
	return &PostgresSubscriptionRepository{pool: pool}
}

func scanPlan(row pgx.Row, p *Plan) error {
	var limitsRaw []byte
	if err := row.Scan(&p.ID, &p.Tier, &p.Price, &p.Features, &limitsRaw); err != nil {
		return err
	}
	p.Limits = map[string]string{}
	if len(limitsRaw) > 0 {
		if err := json.Unmarshal(limitsRaw, &p.Limits); err != nil {
			return fmt.Errorf("decode plan limits: %w", err)
		}
	}
	return nil
}

func (r *PostgresSubscriptionRepository) ListPlans(ctx context.Context) ([]Plan, error) {
	const q = `SELECT id, tier, price, features, limits FROM subscription_plans ORDER BY tier ASC`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := scanPlan(rows, &p); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresSubscriptionRepository) GetPlanByID(ctx context.Context, id string) (Plan, error) {
	const q = `SELECT id, tier, price, features, limits FROM subscription_plans WHERE id = $1`
	var p Plan
	if err := scanPlan(r.pool.QueryRow(ctx, q, id), &p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Plan{}, ErrPlanNotFound
		}
		return Plan{}, fmt.Errorf("get plan by id %q: %w", id, err)
	}
	return p, nil
}

func (r *PostgresSubscriptionRepository) UpsertSubscription(ctx context.Context, sub SellerSubscription) (SellerSubscription, error) {
	if sub.ID == "" {
		sub.ID = newID()
	}
	if sub.Status == "" {
		sub.Status = SubscriptionStatusActive
	}
	// One subscription per seller: replace plan/status on conflict, keeping the
	// original id and created_at.
	const q = `INSERT INTO seller_subscriptions (id, seller_id, plan_id, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (seller_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, status = EXCLUDED.status
		RETURNING id, seller_id, plan_id, status, created_at`
	var out SellerSubscription
	if err := r.pool.QueryRow(ctx, q, sub.ID, sub.SellerID, sub.PlanID, sub.Status).Scan(
		&out.ID, &out.SellerID, &out.PlanID, &out.Status, &out.CreatedAt,
	); err != nil {
		return SellerSubscription{}, fmt.Errorf("upsert subscription: %w", err)
	}
	return out, nil
}

func (r *PostgresSubscriptionRepository) GetActiveSubscriptionBySeller(ctx context.Context, sellerID string) (SellerSubscription, error) {
	const q = `SELECT s.id, s.seller_id, s.plan_id, p.tier, s.status, s.created_at
		FROM seller_subscriptions s
		JOIN subscription_plans p ON p.id = s.plan_id
		WHERE s.seller_id = $1 AND s.status = $2`
	var out SellerSubscription
	if err := r.pool.QueryRow(ctx, q, sellerID, SubscriptionStatusActive).Scan(
		&out.ID, &out.SellerID, &out.PlanID, &out.Tier, &out.Status, &out.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SellerSubscription{}, ErrSubscriptionNotFound
		}
		return SellerSubscription{}, fmt.Errorf("get subscription for seller %q: %w", sellerID, err)
	}
	return out, nil
}

func (r *PostgresSubscriptionRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// ── In-memory implementation ──

// InMemorySubscriptionRepository is the DB-less backend used when
// DATABASE_ENABLED=false and in unit tests. Plans are seeded from DefaultPlans.
type InMemorySubscriptionRepository struct {
	mu    sync.RWMutex
	plans map[string]Plan               // keyed by id
	subs  map[string]SellerSubscription // keyed by seller_id (one per seller)
}

func NewInMemorySubscriptionRepository() *InMemorySubscriptionRepository {
	plans := make(map[string]Plan)
	for _, p := range DefaultPlans() {
		plans[p.ID] = p
	}
	return &InMemorySubscriptionRepository{
		plans: plans,
		subs:  make(map[string]SellerSubscription),
	}
}

func (r *InMemorySubscriptionRepository) ListPlans(_ context.Context) ([]Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plan, 0, len(r.plans))
	for _, p := range r.plans {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tier < out[j].Tier })
	return out, nil
}

func (r *InMemorySubscriptionRepository) GetPlanByID(_ context.Context, id string) (Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return p, nil
}

func (r *InMemorySubscriptionRepository) UpsertSubscription(_ context.Context, sub SellerSubscription) (SellerSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sub.Status == "" {
		sub.Status = SubscriptionStatusActive
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
		sub.ID = newID()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	r.subs[sub.SellerID] = sub
	return sub, nil
}

func (r *InMemorySubscriptionRepository) GetActiveSubscriptionBySeller(_ context.Context, sellerID string) (SellerSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub, ok := r.subs[sellerID]
	if !ok || sub.Status != SubscriptionStatusActive {
		return SellerSubscription{}, ErrSubscriptionNotFound
	}
	// Denormalize the current plan tier in (mirrors the Postgres join).
	if p, ok := r.plans[sub.PlanID]; ok {
		sub.Tier = p.Tier
	}
	return sub, nil
}

func (r *InMemorySubscriptionRepository) Ping(_ context.Context) error { return nil }
