package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/alerts"
	"github.com/buidangphuc/team-notification/internal/inbox"
	"github.com/buidangphuc/team-notification/internal/prefs"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Inbox interface {
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error)
	MarkAsRead(ctx context.Context, id, userID string) error
	GetUnreadCount(ctx context.Context, userID string) (int32, error)
}

type Handler struct {
	notificationv1.UnimplementedNotificationServiceServer
	inbox  Inbox
	alerts *alerts.Service
	prefs  *prefs.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(in Inbox, al *alerts.Service, pr *prefs.Service, logger *slog.Logger) (*Handler, error) {
	if in == nil {
		return nil, errors.New("rpc: inbox is required")
	}
	if al == nil {
		return nil, errors.New("rpc: alerts service is required")
	}
	if pr == nil {
		return nil, errors.New("rpc: prefs service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: alerts.ErrEmptyUser, Code: codes.InvalidArgument},
		grpcx.Rule{Target: alerts.ErrEmptyListing, Code: codes.InvalidArgument},
		grpcx.Rule{Target: alerts.ErrInvalidType, Code: codes.InvalidArgument},
		grpcx.Rule{Target: alerts.ErrEmptySubID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: prefs.ErrNilPrefs, Code: codes.InvalidArgument},
		grpcx.Rule{Target: inbox.ErrNotFound, Code: codes.NotFound, Message: "notification not found"},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{inbox: in, alerts: al, prefs: pr, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	notificationv1.RegisterNotificationServiceServer(srv, h)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func (h *Handler) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	size := int(req.GetPageSize())
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	page := int(req.GetPageNumber())
	if page <= 0 {
		page = 1
	}
	rows, unread, err := h.inbox.ListNotifications(ctx, p.ID, size, (page-1)*size)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.ListNotificationsResponse{Notifications: rows, TotalUnread: unread}, nil
}

func (h *Handler) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*notificationv1.MarkAsReadResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.inbox.MarkAsRead(ctx, req.GetId(), p.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.MarkAsReadResponse{Success: true}, nil
}

func (h *Handler) GetUnreadCount(ctx context.Context, _ *notificationv1.GetUnreadCountRequest) (*notificationv1.GetUnreadCountResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.inbox.GetUnreadCount(ctx, p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.GetUnreadCountResponse{UnreadCount: n}, nil
}

func (h *Handler) SubscribeAlert(ctx context.Context, req *notificationv1.SubscribeAlertRequest) (*notificationv1.SubscribeAlertResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := h.alerts.Subscribe(ctx, p.ID, req.GetListingId(), req.GetType())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.SubscribeAlertResponse{Subscription: sub}, nil
}

func (h *Handler) UnsubscribeAlert(ctx context.Context, req *notificationv1.UnsubscribeAlertRequest) (*notificationv1.UnsubscribeAlertResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.alerts.Unsubscribe(ctx, p.ID, req.GetSubscriptionId()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.UnsubscribeAlertResponse{}, nil
}

func (h *Handler) ListAlertSubscriptions(ctx context.Context, _ *notificationv1.ListAlertSubscriptionsRequest) (*notificationv1.ListAlertSubscriptionsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := h.alerts.List(ctx, p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.ListAlertSubscriptionsResponse{Subscriptions: subs}, nil
}

func (h *Handler) GetNotificationPrefs(ctx context.Context, _ *notificationv1.GetNotificationPrefsRequest) (*notificationv1.GetNotificationPrefsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.prefs.Get(ctx, p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if out == nil {
		out = prefs.DefaultPrefs()
	}
	return &notificationv1.GetNotificationPrefsResponse{Prefs: out}, nil
}

func (h *Handler) UpdateNotificationPrefs(ctx context.Context, req *notificationv1.UpdateNotificationPrefsRequest) (*notificationv1.UpdateNotificationPrefsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.prefs.Update(ctx, p.ID, req.GetPrefs())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &notificationv1.UpdateNotificationPrefsResponse{Prefs: out}, nil
}
