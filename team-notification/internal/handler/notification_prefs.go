package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/service"
)

// prefsUserID resolves the caller's identity. In production this is read from
// the principal in context (ADR-0003); mirror notification.go's placeholder
// until the interceptor is wired so preferences attach to the same demo user.
func (h *NotificationHandler) prefsUserID(_ context.Context) string {
	return "khach_hang_shopee"
}

// mapPrefsErr turns a service validation error into the right gRPC status.
func mapPrefsErr(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyUser),
		errors.Is(err, service.ErrNilPrefs):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

func (h *NotificationHandler) GetNotificationPrefs(ctx context.Context, _ *notificationv1.GetNotificationPrefsRequest) (*notificationv1.GetNotificationPrefsResponse, error) {
	if h.prefs == nil {
		return nil, status.Error(codes.Unimplemented, "notification prefs not enabled")
	}
	prefs, err := h.prefs.Get(ctx, h.prefsUserID(ctx))
	if err != nil {
		return nil, mapPrefsErr(err)
	}
	return &notificationv1.GetNotificationPrefsResponse{Prefs: prefs}, nil
}

func (h *NotificationHandler) UpdateNotificationPrefs(ctx context.Context, req *notificationv1.UpdateNotificationPrefsRequest) (*notificationv1.UpdateNotificationPrefsResponse, error) {
	if h.prefs == nil {
		return nil, status.Error(codes.Unimplemented, "notification prefs not enabled")
	}
	prefs, err := h.prefs.Update(ctx, h.prefsUserID(ctx), req.GetPrefs())
	if err != nil {
		return nil, mapPrefsErr(err)
	}
	return &notificationv1.UpdateNotificationPrefsResponse{Prefs: prefs}, nil
}
