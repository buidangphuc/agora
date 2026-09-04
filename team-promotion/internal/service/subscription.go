package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

// ErrPlanNotFound is returned by Subscribe for an unknown plan id.
var ErrPlanNotFound = errors.New("plan not found")

// ErrInvalidSubscription is returned for malformed Subscribe input.
var ErrInvalidSubscription = errors.New("invalid subscription")

// SubscriptionService holds the plan-catalog + seller-subscription business logic.
// Subscriptions are a MOCK ledger: Subscribe records the chosen tier but never
// moves money (payments/wallet stay mock, AGENTS.md §7).
type SubscriptionService struct {
	repo   repository.SubscriptionRepository
	logger *slog.Logger
	nowFn  func() time.Time
}

// NewSubscriptionService wires the subscription repository.
func NewSubscriptionService(repo repository.SubscriptionRepository, logger *slog.Logger) *SubscriptionService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SubscriptionService{repo: repo, logger: logger, nowFn: time.Now}
}

// ListPlans returns the FREE/PRO/PREMIUM catalog (public reference data).
func (s *SubscriptionService) ListPlans(ctx context.Context) ([]repository.Plan, error) {
	return s.repo.ListPlans(ctx)
}

// Subscribe records that a seller has taken a plan. MOCK: no charge — it validates
// the plan exists and upserts an active subscription (one per seller; re-subscribing
// switches the plan). Returns the stored subscription with its tier populated.
func (s *SubscriptionService) Subscribe(ctx context.Context, sellerID, planID string) (repository.SellerSubscription, error) {
	if strings.TrimSpace(sellerID) == "" {
		return repository.SellerSubscription{}, ErrInvalidSubscription
	}
	if strings.TrimSpace(planID) == "" {
		return repository.SellerSubscription{}, ErrInvalidSubscription
	}
	plan, err := s.repo.GetPlanByID(ctx, planID)
	if err != nil {
		if errors.Is(err, repository.ErrPlanNotFound) {
			return repository.SellerSubscription{}, ErrPlanNotFound
		}
		return repository.SellerSubscription{}, err
	}
	sub, err := s.repo.UpsertSubscription(ctx, repository.SellerSubscription{
		SellerID:  sellerID,
		PlanID:    plan.ID,
		Tier:      plan.Tier,
		Status:    repository.SubscriptionStatusActive,
		CreatedAt: s.nowFn(),
	})
	if err != nil {
		return repository.SellerSubscription{}, err
	}
	// Ensure the returned tier reflects the plan even if the repo left it unset.
	sub.Tier = plan.Tier
	return sub, nil
}

// GetEntitlements returns a seller's current plan tier and entitlement limits.
// A seller with no active subscription (or an empty seller id) defaults to FREE —
// never panics, never errors on the "unsubscribed" path.
func (s *SubscriptionService) GetEntitlements(ctx context.Context, sellerID string) (int32, map[string]string, error) {
	free := repository.FreePlan()
	if strings.TrimSpace(sellerID) == "" {
		return free.Tier, free.Limits, nil
	}
	sub, err := s.repo.GetActiveSubscriptionBySeller(ctx, sellerID)
	if err != nil {
		if errors.Is(err, repository.ErrSubscriptionNotFound) {
			return free.Tier, free.Limits, nil
		}
		return 0, nil, err
	}
	plan, err := s.repo.GetPlanByID(ctx, sub.PlanID)
	if err != nil {
		if errors.Is(err, repository.ErrPlanNotFound) {
			// Subscription points at a plan that no longer exists — fall back to FREE.
			return free.Tier, free.Limits, nil
		}
		return 0, nil, err
	}
	return plan.Tier, plan.Limits, nil
}
