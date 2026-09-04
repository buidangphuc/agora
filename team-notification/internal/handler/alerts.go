package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/service"
)

// alertUserID resolves the caller's identity. In production this is read from the
// principal in context (ADR-0003); mirror notification.go's placeholder until the
// interceptor is wired so alert subscriptions attach to the same demo user.
func (h *NotificationHandler) alertUserID(_ context.Context) string {
	return "khach_hang_shopee"
}

// mapAlertErr turns a service validation error into the right gRPC status.
func mapAlertErr(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyListing),
		errors.Is(err, service.ErrInvalidType),
		errors.Is(err, service.ErrEmptySubID),
		errors.Is(err, service.ErrEmptyUser):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

func (h *NotificationHandler) SubscribeAlert(ctx context.Context, req *notificationv1.SubscribeAlertRequest) (*notificationv1.SubscribeAlertResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	sub, err := h.alerts.Subscribe(ctx, h.alertUserID(ctx), req.GetListingId(), req.GetType())
	if err != nil {
		return nil, mapAlertErr(err)
	}
	return &notificationv1.SubscribeAlertResponse{Subscription: sub}, nil
}

func (h *NotificationHandler) UnsubscribeAlert(ctx context.Context, req *notificationv1.UnsubscribeAlertRequest) (*notificationv1.UnsubscribeAlertResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	if err := h.alerts.Unsubscribe(ctx, h.alertUserID(ctx), req.GetSubscriptionId()); err != nil {
		return nil, mapAlertErr(err)
	}
	return &notificationv1.UnsubscribeAlertResponse{}, nil
}

func (h *NotificationHandler) ListAlertSubscriptions(ctx context.Context, _ *notificationv1.ListAlertSubscriptionsRequest) (*notificationv1.ListAlertSubscriptionsResponse, error) {
	if h.alerts == nil {
		return nil, status.Error(codes.Unimplemented, "alert subscriptions not enabled")
	}
	subs, err := h.alerts.List(ctx, h.alertUserID(ctx))
	if err != nil {
		return nil, mapAlertErr(err)
	}
	return &notificationv1.ListAlertSubscriptionsResponse{Subscriptions: subs}, nil
}
