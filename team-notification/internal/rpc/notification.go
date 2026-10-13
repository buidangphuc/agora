// Package rpc is the transport layer: it decodes requests, reads the caller from
// the principal the SDK auth interceptor resolved, calls the use cases and maps
// their errors to gRPC status.
package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/notification"
	"github.com/buidangphuc/team-notification/internal/store"
)

// NotificationHandler implements platform.notification.v1.NotificationService.
type NotificationHandler struct {
	notificationv1.UnimplementedNotificationServiceServer
	repo   store.NotificationRepository
	alerts *notification.AlertService
	prefs  *notification.PrefsService
}

// Register adds the notification service to s.
func Register(s grpc.ServiceRegistrar, h *NotificationHandler) {
	notificationv1.RegisterNotificationServiceServer(s, h)
}

// Option customizes a NotificationHandler at construction without breaking the
// positional constructor (existing call sites keep compiling).
type Option func(*NotificationHandler)

// WithAlertService wires the alert-subscription use case, enabling the
// SubscribeAlert/UnsubscribeAlert/ListAlertSubscriptions RPCs. When unset those
// RPCs return Unimplemented (via the embedded server).
func WithAlertService(alerts *notification.AlertService) Option {
	return func(h *NotificationHandler) { h.alerts = alerts }
}

// WithPrefsService wires the notification-preferences use case, enabling the
// GetNotificationPrefs/UpdateNotificationPrefs RPCs. When unset those RPCs
// return Unimplemented (via the embedded server).
func WithPrefsService(prefs *notification.PrefsService) Option {
	return func(h *NotificationHandler) { h.prefs = prefs }
}

// NewNotificationHandler returns a handler over repo; opts wire the alert and
// preferences use cases.
func NewNotificationHandler(repo store.NotificationRepository, opts ...Option) *NotificationHandler {
	h := &NotificationHandler{repo: repo}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ListNotifications returns the caller's notifications, newest first, paged.
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
		return nil, internalErr(ctx, "list notifications", err)
	}

	return &notificationv1.ListNotificationsResponse{
		Notifications: notis,
		TotalUnread:   unread,
	}, nil
}

// MarkAsRead marks one of the caller's notifications read, or all of them when
// the id is empty.
func (h *NotificationHandler) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*notificationv1.MarkAsReadResponse, error) {
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.repo.MarkAsRead(ctx, req.GetId(), userID); err != nil {
		return nil, grpcx.Status(ctx, slog.With("op", "mark as read"), err, notificationErrs)
	}
	return &notificationv1.MarkAsReadResponse{Success: true}, nil
}

// GetUnreadCount returns the caller's unread count.
func (h *NotificationHandler) GetUnreadCount(ctx context.Context, _ *notificationv1.GetUnreadCountRequest) (*notificationv1.GetUnreadCountResponse, error) {
	userID, err := callerUserID(ctx)
	if err != nil {
		return nil, err
	}
	count, err := h.repo.GetUnreadCount(ctx, userID)
	if err != nil {
		return nil, internalErr(ctx, "get unread count", err)
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
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return "", err
	}
	if p.Type != auth.TypeUser {
		return "", status.Error(codes.PermissionDenied, "per-user notification RPCs require a user principal")
	}
	return p.ID, nil
}
