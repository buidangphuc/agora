package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/service"
)

// SubscriptionHandler serves platform.promotion.v1.SubscriptionService: the plan
// catalog, mock Subscribe, and per-tier entitlement lookup. No money moves here
// (AGENTS.md §7).
type SubscriptionHandler struct {
	promotionv1.UnimplementedSubscriptionServiceServer

	svc    *service.SubscriptionService
	logger *slog.Logger
}

func NewSubscriptionHandler(svc *service.SubscriptionService, logger *slog.Logger) *SubscriptionHandler {
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
		return nil, status.Errorf(codes.Internal, "list plans: %v", err)
	}
	out := make([]*promotionv1.Plan, 0, len(plans))
	for _, p := range plans {
		out = append(out, service.PlanToProto(p))
	}
	return &promotionv1.ListPlansResponse{Plans: out}, nil
}

// Subscribe records the authenticated seller onto a plan (MOCK — no charge). The
// seller id is bound from the principal, never the wire, so a caller can only
// subscribe themselves.
func (h *SubscriptionHandler) Subscribe(ctx context.Context, req *promotionv1.SubscribeRequest) (*promotionv1.SubscribeResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetPlanId() == "" {
		return nil, status.Error(codes.InvalidArgument, "plan_id is required")
	}
	sub, err := h.svc.Subscribe(ctx, principal.GetId(), req.GetPlanId())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlanNotFound):
			return nil, status.Error(codes.NotFound, "plan not found")
		case errors.Is(err, service.ErrInvalidSubscription):
			return nil, status.Error(codes.InvalidArgument, "invalid subscription")
		default:
			return nil, status.Errorf(codes.Internal, "subscribe: %v", err)
		}
	}
	return &promotionv1.SubscribeResponse{Subscription: service.SubscriptionToProto(sub)}, nil
}

// GetEntitlements returns the current plan tier + limits for a seller, defaulting
// to FREE when unsubscribed. Auth-scoped: a USER principal may only read their own
// entitlements; a SERVICE principal (e.g. the order saga) may read any seller.
func (h *SubscriptionHandler) GetEntitlements(ctx context.Context, req *promotionv1.GetEntitlementsRequest) (*promotionv1.GetEntitlementsResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.GetId()
	}
	// Cross-user isolation: a user cannot read another seller's entitlements.
	if principal.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_USER && sellerID != principal.GetId() {
		return nil, status.Error(codes.PermissionDenied, "cannot read another seller's entitlements")
	}
	tier, limits, err := h.svc.GetEntitlements(ctx, sellerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get entitlements: %v", err)
	}
	return &promotionv1.GetEntitlementsResponse{
		Plan:   promotionv1.PlanTier(tier),
		Limits: limits,
	}, nil
}
