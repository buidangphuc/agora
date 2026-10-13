package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/promotion"
)

// SubscriptionHandler serves platform.promotion.v1.SubscriptionService: the plan
// catalog, mock Subscribe, and per-tier entitlement lookup. No money moves here
// (AGENTS.md §7).
type SubscriptionHandler struct {
	promotionv1.UnimplementedSubscriptionServiceServer

	svc    *promotion.SubscriptionService
	logger *slog.Logger
}

// NewSubscriptionHandler constructs the value of the same name.
func NewSubscriptionHandler(svc *promotion.SubscriptionService, logger *slog.Logger) *SubscriptionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SubscriptionHandler{svc: svc, logger: logger}
}

// ListPlans is a public read of the FREE/PRO/PREMIUM catalog (no auth required —
// mirrors GetVoucher's unauthenticated catalog read).
func (h *SubscriptionHandler) ListPlans(ctx context.Context, _ *promotionv1.ListPlansRequest) (*promotionv1.ListPlansResponse, error) {
	plans, err := h.svc.ListPlans(ctx)
	if err != nil {
		return nil, internalError(ctx, h.logger, "list plans", err)
	}
	out := make([]*promotionv1.Plan, 0, len(plans))
	for _, p := range plans {
		out = append(out, promotion.PlanToProto(p))
	}
	return &promotionv1.ListPlansResponse{Plans: out}, nil
}

// Subscribe records the authenticated seller onto a plan (MOCK — no charge). Plans
// are seller plans: requires listing.write (seller, admin); a buyer-only principal
// is PERMISSION_DENIED and nothing is stored. The seller id is bound from the
// principal, never the wire, so a caller can only subscribe themselves.
func (h *SubscriptionHandler) Subscribe(ctx context.Context, req *promotionv1.SubscribeRequest) (*promotionv1.SubscribeResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id is required")
	}
	sub, err := h.svc.Subscribe(ctx, principal.ID, req.GetPlanId())
	if err != nil {
		return nil, internalError(ctx, h.logger, "subscribe", err)
	}
	return &promotionv1.SubscribeResponse{Subscription: promotion.SubscriptionToProto(sub)}, nil
}

// GetEntitlements returns the current plan tier + limits for a seller, defaulting
// to FREE when unsubscribed. Auth-scoped: only a SERVICE principal (e.g. the order
// saga) may read another seller; every other principal type (user, unspecified)
// may read only its own. The anonymous principal is UNAUTHENTICATED.
func (h *SubscriptionHandler) GetEntitlements(ctx context.Context, req *promotionv1.GetEntitlementsRequest) (*promotionv1.GetEntitlementsResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.ID
	}
	// Cross-seller isolation, fail closed: anything that is not a service principal
	// may read only its own entitlements.
	if !isService(principal) && sellerID != principal.ID {
		return nil, status.Error(codes.PermissionDenied, "cannot read another seller's entitlements")
	}
	tier, limits, err := h.svc.GetEntitlements(ctx, sellerID)
	if err != nil {
		return nil, internalError(ctx, h.logger, "get entitlements", err)
	}
	return &promotionv1.GetEntitlementsResponse{
		Plan:   promotionv1.PlanTier(tier),
		Limits: limits,
	}, nil
}
