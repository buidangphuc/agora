package subscription

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

// ErrInvalidSubscription is returned for malformed Subscribe input.
var ErrInvalidSubscription = errors.New("invalid subscription")

type Service struct {
	repo   store.SubscriptionRepository
	logger *slog.Logger
	nowFn  func() time.Time
}

func New(repo store.SubscriptionRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger, nowFn: time.Now}
}

// ListPlans returns the FREE/PRO/PREMIUM catalog (public reference data).
func (s *Service) ListPlans(ctx context.Context) ([]store.Plan, error) {
	return s.repo.ListPlans(ctx)
}

// Subscribe records that a seller has taken a plan. MOCK: no charge — it validates
// the plan exists and upserts an active subscription (one per seller; re-subscribing
// switches the plan). Returns the stored subscription with its tier populated.
func (s *Service) Subscribe(ctx context.Context, sellerID, planID string) (store.SellerSubscription, error) {
	if strings.TrimSpace(sellerID) == "" {
		return store.SellerSubscription{}, ErrInvalidSubscription
	}
	if strings.TrimSpace(planID) == "" {
		return store.SellerSubscription{}, ErrInvalidSubscription
	}
	plan, err := s.repo.GetPlanByID(ctx, planID)
	if err != nil {
		return store.SellerSubscription{}, err
	}
	sub, err := s.repo.UpsertSubscription(ctx, store.SellerSubscription{
		SellerID:  sellerID,
		PlanID:    plan.ID,
		Tier:      plan.Tier,
		Status:    store.SubscriptionStatusActive,
		CreatedAt: s.nowFn(),
	})
	if err != nil {
		return store.SellerSubscription{}, err
	}
	// Ensure the returned tier reflects the plan even if the repo left it unset.
	sub.Tier = plan.Tier
	return sub, nil
}

// GetEntitlements returns a seller's current plan tier and entitlement limits.
// A seller with no active subscription (or an empty seller id) defaults to FREE —
// never panics, never errors on the "unsubscribed" path.
func (s *Service) GetEntitlements(ctx context.Context, sellerID string) (int32, map[string]string, error) {
	free := store.FreePlan()
	if strings.TrimSpace(sellerID) == "" {
		return free.Tier, free.Limits, nil
	}
	sub, err := s.repo.GetActiveSubscriptionBySeller(ctx, sellerID)
	if err != nil {
		if errors.Is(err, store.ErrSubscriptionNotFound) {
			return free.Tier, free.Limits, nil
		}
		return 0, nil, err
	}
	plan, err := s.repo.GetPlanByID(ctx, sub.PlanID)
	if err != nil {
		if errors.Is(err, store.ErrPlanNotFound) {
			// Subscription points at a plan that no longer exists — fall back to FREE.
			return free.Tier, free.Limits, nil
		}
		return 0, nil, err
	}
	return plan.Tier, plan.Limits, nil
}
