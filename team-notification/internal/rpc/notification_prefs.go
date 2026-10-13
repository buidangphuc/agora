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

var prefsErrs = mustTable(
	grpcx.Rule{Target: notification.ErrEmptyUser, Code: codes.InvalidArgument},
	grpcx.Rule{Target: notification.ErrNilPrefs, Code: codes.InvalidArgument},
)

// mapPrefsErr turns a service validation error into the right gRPC status.
func mapPrefsErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, slog.With("op", "service call"), err, prefsErrs)
}

// GetNotificationPrefs returns the caller's preferences, or the defaults.
func (h *NotificationHandler) GetNotificationPrefs(ctx context.Context, _ *notificationv1.GetNotificationPrefsRequest) (*notificationv1.GetNotificationPrefsResponse, error) {
	if h.prefs == nil {
		return nil, status.Error(codes.Unimplemented, "notification prefs not enabled")
	}
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := h.prefs.Get(ctx, userID)
	if err != nil {
		return nil, mapPrefsErr(ctx, err)
	}
	return &notificationv1.GetNotificationPrefsResponse{Prefs: prefs}, nil
}

// UpdateNotificationPrefs stores the caller's preferences.
func (h *NotificationHandler) UpdateNotificationPrefs(ctx context.Context, req *notificationv1.UpdateNotificationPrefsRequest) (*notificationv1.UpdateNotificationPrefsResponse, error) {
	if h.prefs == nil {
		return nil, status.Error(codes.Unimplemented, "notification prefs not enabled")
	}
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := h.prefs.Update(ctx, userID, req.GetPrefs())
	if err != nil {
		return nil, mapPrefsErr(ctx, err)
	}
	return &notificationv1.UpdateNotificationPrefsResponse{Prefs: prefs}, nil
}
