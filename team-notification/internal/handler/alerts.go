package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/service"
)

// mapAlertErr turns a service validation error into the right gRPC status.
func mapAlertErr(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyListing),
		errors.Is(err, service.ErrInvalidType),
		errors.Is(err, service.ErrEmptySubID),
		errors.Is(err, service.ErrEmptyUser):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return internalErr(ctx, "service call", err)
	}
}

func (h *NotificationHandler) SubscribeAlert(ctx context.Context, req *notificationv1.SubscribeAlertRequest) (*notificationv1.SubscribeAlertResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := h.alerts.Subscribe(ctx, userID, req.GetListingId(), req.GetType())
	if err != nil {
		return nil, mapAlertErr(ctx, err)
	}
	return &notificationv1.SubscribeAlertResponse{Subscription: sub}, nil
}

func (h *NotificationHandler) UnsubscribeAlert(ctx context.Context, req *notificationv1.UnsubscribeAlertRequest) (*notificationv1.UnsubscribeAlertResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.alerts.Unsubscribe(ctx, userID, req.GetSubscriptionId()); err != nil {
		return nil, mapAlertErr(ctx, err)
	}
	return &notificationv1.UnsubscribeAlertResponse{}, nil
}

func (h *NotificationHandler) ListAlertSubscriptions(ctx context.Context, _ *notificationv1.ListAlertSubscriptionsRequest) (*notificationv1.ListAlertSubscriptionsResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.alerts.List(ctx, userID)
	if err != nil {
		return nil, mapAlertErr(ctx, err)
	}
	return &notificationv1.ListAlertSubscriptionsResponse{Subscriptions: subs}, nil
}
