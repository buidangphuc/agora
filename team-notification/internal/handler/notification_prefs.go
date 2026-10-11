package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/service"
)

// mapPrefsErr turns a service validation error into the right gRPC status.
func mapPrefsErr(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyUser),
		errors.Is(err, service.ErrNilPrefs):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return internalErr(ctx, "service call", err)
	}
}

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
