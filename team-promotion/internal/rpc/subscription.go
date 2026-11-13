package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/subscription"
)

type SubscriptionHandler struct {
	promotionv1.UnimplementedSubscriptionServiceServer

	svc    *subscription.Service
	logger *slog.Logger
}

func NewSubscriptionHandler(svc *subscription.Service, logger *slog.Logger) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc, logger: orDefault(logger)}
}

func (h *SubscriptionHandler) ListPlans(ctx context.Context, _ *promotionv1.ListPlansRequest) (*promotionv1.ListPlansResponse, error) {
	plans, err := h.svc.ListPlans(ctx)
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	out := make([]*promotionv1.Plan, 0, len(plans))
	for _, p := range plans {
		out = append(out, planToProto(p))
	}
	return &promotionv1.ListPlansResponse{Plans: out}, nil
}

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
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.SubscribeResponse{Subscription: subscriptionToProto(sub)}, nil
}

func (h *SubscriptionHandler) GetEntitlements(ctx context.Context, req *promotionv1.GetEntitlementsRequest) (*promotionv1.GetEntitlementsResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.ID
	}
	if principal.Type != auth.TypeService && sellerID != principal.ID {
		return nil, status.Error(codes.PermissionDenied, "cannot read another seller's entitlements")
	}
	tier, limits, err := h.svc.GetEntitlements(ctx, sellerID)
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.GetEntitlementsResponse{Plan: promotionv1.PlanTier(tier), Limits: limits}, nil
}

func (h *SubscriptionHandler) GetMySubscription(ctx context.Context, _ *promotionv1.GetMySubscriptionRequest) (*promotionv1.GetMySubscriptionResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.GetMySubscription(ctx, principal.ID)
	if err != nil {
		if errors.Is(err, store.ErrSubscriptionNotFound) {
			return &promotionv1.GetMySubscriptionResponse{}, nil
		}
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.GetMySubscriptionResponse{Subscription: subscriptionToProto(sub)}, nil
}

func (h *SubscriptionHandler) CancelSubscription(ctx context.Context, _ *promotionv1.CancelSubscriptionRequest) (*promotionv1.CancelSubscriptionResponse, error) {
	principal, err := auth.RequireScopes(ctx, scopeListingWrite)
	if err != nil {
		return nil, err
	}
	sub, err := h.svc.Cancel(ctx, principal.ID)
	if err != nil {
		return nil, fail(ctx, h.logger, err)
	}
	return &promotionv1.CancelSubscriptionResponse{Subscription: subscriptionToProto(sub)}, nil
}
