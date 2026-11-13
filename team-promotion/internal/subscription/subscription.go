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
	repo    Repository
	payment PaymentCharger
	logger  *slog.Logger
	nowFn   func() time.Time
}

func New(repo Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger, nowFn: time.Now}
}

// ListPlans returns the FREE/PRO/PREMIUM catalog (public reference data).
func (s *Service) ListPlans(ctx context.Context) ([]store.Plan, error) {
	return s.repo.ListPlans(ctx)
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
