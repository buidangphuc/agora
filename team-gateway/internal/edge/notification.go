package edge

import (
	"context"

	"connectrpc.com/connect"

	notificationv1 "github.com/buidangphuc/team-gateway/generated/platform/notification/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/notification/v1/notificationv1connect"
)

// NotificationForwarder forwards notification-bell feed + price-drop/back-in-stock
// alert-subscription calls to team-notification.
type NotificationForwarder struct {
	notificationv1connect.UnimplementedNotificationServiceHandler
	client notificationv1.NotificationServiceClient
	edge   *Edge
}

func NewNotificationForwarder(client notificationv1.NotificationServiceClient, edge *Edge) *NotificationForwarder {
	return &NotificationForwarder{client: client, edge: edge}
}

func (f *NotificationForwarder) ListNotifications(
	ctx context.Context, req *connect.Request[notificationv1.ListNotificationsRequest],
) (*connect.Response[notificationv1.ListNotificationsResponse], error) {
	var out *notificationv1.ListNotificationsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListNotifications(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *NotificationForwarder) MarkAsRead(
	ctx context.Context, req *connect.Request[notificationv1.MarkAsReadRequest],
) (*connect.Response[notificationv1.MarkAsReadResponse], error) {
	var out *notificationv1.MarkAsReadResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.MarkAsRead(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *NotificationForwarder) GetUnreadCount(
	ctx context.Context, req *connect.Request[notificationv1.GetUnreadCountRequest],
) (*connect.Response[notificationv1.GetUnreadCountResponse], error) {
	var out *notificationv1.GetUnreadCountResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetUnreadCount(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Alert Subscriptions ──

func (f *NotificationForwarder) SubscribeAlert(
	ctx context.Context, req *connect.Request[notificationv1.SubscribeAlertRequest],
) (*connect.Response[notificationv1.SubscribeAlertResponse], error) {
	var out *notificationv1.SubscribeAlertResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SubscribeAlert(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *NotificationForwarder) UnsubscribeAlert(
	ctx context.Context, req *connect.Request[notificationv1.UnsubscribeAlertRequest],
) (*connect.Response[notificationv1.UnsubscribeAlertResponse], error) {
	var out *notificationv1.UnsubscribeAlertResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UnsubscribeAlert(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *NotificationForwarder) ListAlertSubscriptions(
	ctx context.Context, req *connect.Request[notificationv1.ListAlertSubscriptionsRequest],
) (*connect.Response[notificationv1.ListAlertSubscriptionsResponse], error) {
	var out *notificationv1.ListAlertSubscriptionsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListAlertSubscriptions(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Notification Preferences ──

func (f *NotificationForwarder) GetNotificationPrefs(
	ctx context.Context, req *connect.Request[notificationv1.GetNotificationPrefsRequest],
) (*connect.Response[notificationv1.GetNotificationPrefsResponse], error) {
	var out *notificationv1.GetNotificationPrefsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetNotificationPrefs(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *NotificationForwarder) UpdateNotificationPrefs(
	ctx context.Context, req *connect.Request[notificationv1.UpdateNotificationPrefsRequest],
) (*connect.Response[notificationv1.UpdateNotificationPrefsResponse], error) {
	var out *notificationv1.UpdateNotificationPrefsResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateNotificationPrefs(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
