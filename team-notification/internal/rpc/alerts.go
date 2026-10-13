package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/notification"
)

var alertErrs = mustTable(
	grpcx.Rule{Target: notification.ErrEmptyListing, Code: codes.InvalidArgument},
	grpcx.Rule{Target: notification.ErrInvalidType, Code: codes.InvalidArgument},
	grpcx.Rule{Target: notification.ErrEmptySubID, Code: codes.InvalidArgument},
	grpcx.Rule{Target: notification.ErrEmptyUser, Code: codes.InvalidArgument},
)

// mapAlertErr turns a service validation error into the right gRPC status.
func mapAlertErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, slog.With("op", "service call"), err, alertErrs)
}

// SubscribeAlert subscribes the caller to a price-drop or back-in-stock alert.
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

// UnsubscribeAlert removes one of the caller's alert subscriptions.
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

// ListAlertSubscriptions returns the caller's alert subscriptions.
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
