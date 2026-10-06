package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-notification/generated/platform/common/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/interceptor"
	"github.com/buidangphuc/team-notification/internal/repository"
	"github.com/buidangphuc/team-notification/internal/service"
)

type NotificationHandler struct {
	notificationv1.UnimplementedNotificationServiceServer
	repo   repository.NotificationRepository
	alerts *service.AlertService
	prefs  *service.PrefsService
}

// Option customizes a NotificationHandler at construction without breaking the
// positional constructor (existing call sites keep compiling).
type Option func(*NotificationHandler)

// WithAlertService wires the alert-subscription use case, enabling the
// SubscribeAlert/UnsubscribeAlert/ListAlertSubscriptions RPCs. When unset those
// RPCs return Unimplemented (via the embedded server).
func WithAlertService(alerts *service.AlertService) Option {
	return func(h *NotificationHandler) { h.alerts = alerts }
}

// WithPrefsService wires the notification-preferences use case, enabling the
// GetNotificationPrefs/UpdateNotificationPrefs RPCs. When unset those RPCs
// return Unimplemented (via the embedded server).
func WithPrefsService(prefs *service.PrefsService) Option {
	return func(h *NotificationHandler) { h.prefs = prefs }
}

func NewNotificationHandler(repo repository.NotificationRepository, opts ...Option) *NotificationHandler {
	h := &NotificationHandler{repo: repo}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *NotificationHandler) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}
	pageNumber := int(req.GetPageNumber())
	if pageNumber <= 0 {
		pageNumber = 1
	}
	offset := (pageNumber - 1) * pageSize

	notis, unread, err := h.repo.ListNotifications(ctx, userID, pageSize, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list notifications failed: %v", err)
	}

	return &notificationv1.ListNotificationsResponse{
		Notifications: notis,
		TotalUnread:   unread,
	}, nil
}

func (h *NotificationHandler) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*notificationv1.MarkAsReadResponse, error) {
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.repo.MarkAsRead(ctx, req.GetId(), userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "notification not found")
		}
		return nil, status.Errorf(codes.Internal, "mark as read failed: %v", err)
	}
	return &notificationv1.MarkAsReadResponse{Success: true}, nil
}

func (h *NotificationHandler) GetUnreadCount(ctx context.Context, req *notificationv1.GetUnreadCountRequest) (*notificationv1.GetUnreadCountResponse, error) {
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	count, err := h.repo.GetUnreadCount(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get unread count failed: %v", err)
	}
	return &notificationv1.GetUnreadCountResponse{UnreadCount: count}, nil
}

// callerUserID resolves the per-user owner of an inbox / subscription / prefs
// row from the gateway-forwarded Principal (ADR-0003). Every per-user RPC calls
// it first, so no RPC can run without an owner:
//   - no principal, an anonymous one, or an empty id -> Unauthenticated
//   - a service principal -> PermissionDenied (no internal caller uses these
//     per-user RPCs; the Kafka consumer writes through the repository directly)
//   - a user principal -> its id
func callerUserID(ctx context.Context) (string, error) {
	p, ok := interceptor.PrincipalFromContext(ctx)
	if !ok || p.GetId() == "" || p.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	if p.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		return "", status.Error(codes.PermissionDenied, "per-user notification RPCs require a user principal")
	}
	return p.GetId(), nil
}
