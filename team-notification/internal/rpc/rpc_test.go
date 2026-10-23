package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// Scenario: A user sees only their own notifications
func TestUsersDoNotSeeEachOthersData(t *testing.T) {
	e := newEnv(t)
	alice, bob := asUser("user_alice"), asUser("user_bob")
	seed(e.inbox, "user_alice", "a1", "a2")
	seed(e.inbox, "user_bob", "b1", "b2")

	res, err := e.h.ListNotifications(alice, &notificationv1.ListNotificationsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notifications) != 2 || res.Notifications[0].Id != "a2" || res.Notifications[1].Id != "a1" || res.TotalUnread != 2 {
		t.Fatalf("alice sees %v unread=%d", res.Notifications, res.TotalUnread)
	}
	for _, n := range res.Notifications {
		if n.UserId != "user_alice" {
			t.Fatalf("alice got %s's row", n.UserId)
		}
	}
	cnt, err := e.h.GetUnreadCount(alice, &notificationv1.GetUnreadCountRequest{})
	if err != nil || cnt.UnreadCount != 2 {
		t.Fatalf("alice unread = %v err=%v", cnt.GetUnreadCount(), err)
	}

	if _, err := e.h.SubscribeAlert(alice, &notificationv1.SubscribeAlertRequest{ListingId: "lst_1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP}); err != nil {
		t.Fatal(err)
	}
	a, _ := e.h.ListAlertSubscriptions(alice, &notificationv1.ListAlertSubscriptionsRequest{})
	b, _ := e.h.ListAlertSubscriptions(bob, &notificationv1.ListAlertSubscriptionsRequest{})
	if len(a.Subscriptions) != 1 || len(b.Subscriptions) != 0 {
		t.Fatalf("alice=%d bob=%d subscriptions", len(a.Subscriptions), len(b.Subscriptions))
	}

	if _, err := e.h.UpdateNotificationPrefs(alice, &notificationv1.UpdateNotificationPrefsRequest{
		Prefs: &notificationv1.NotificationPrefs{DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY},
	}); err != nil {
		t.Fatal(err)
	}
	pa, _ := e.h.GetNotificationPrefs(alice, &notificationv1.GetNotificationPrefsRequest{})
	pb, _ := e.h.GetNotificationPrefs(bob, &notificationv1.GetNotificationPrefsRequest{})
	if pa.Prefs.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY {
		t.Fatalf("alice digest = %v", pa.Prefs.DigestFreq)
	}
	if pb.Prefs.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("bob inherited alice's digest: %v", pb.Prefs.DigestFreq)
	}
}

// Scenario: Another user's notification id is NotFound
func TestMarkAsReadOtherUsersNotificationIsNotFound(t *testing.T) {
	e := newEnv(t)
	seed(e.inbox, "user_alice", "n_alice")
	_, err := e.h.MarkAsRead(asUser("user_bob"), &notificationv1.MarkAsReadRequest{Id: "n_alice"})
	wantCode(t, "bob marks alice's", err, codes.NotFound)
	if e.inbox.read("n_alice") {
		t.Fatal("bob marked alice's notification read")
	}
	if _, err := e.h.MarkAsRead(asUser("user_alice"), &notificationv1.MarkAsReadRequest{Id: "n_alice"}); err != nil {
		t.Fatalf("owner mark: %v", err)
	}
	if !e.inbox.read("n_alice") {
		t.Fatal("owner mark did not take effect")
	}
}

// Scenario: An empty id marks all of the caller's notifications read
func TestMarkAsReadEmptyIDMarksAllOfCallerOnly(t *testing.T) {
	e := newEnv(t)
	u1, u2 := asUser("u1"), asUser("u2")
	seed(e.inbox, "u1", "a1", "a2", "a3")
	seed(e.inbox, "u2", "b1", "b2")
	if _, err := e.h.MarkAsRead(u1, &notificationv1.MarkAsReadRequest{}); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	c1, _ := e.h.GetUnreadCount(u1, &notificationv1.GetUnreadCountRequest{})
	c2, _ := e.h.GetUnreadCount(u2, &notificationv1.GetUnreadCountRequest{})
	if c1.UnreadCount != 0 || c2.UnreadCount != 2 {
		t.Fatalf("u1 unread=%d u2 unread=%d, want 0 and 2", c1.UnreadCount, c2.UnreadCount)
	}
}

// Scenario: Principal types are enforced on every per-user RPC
func TestPerUserRPCsRequireAUserPrincipal(t *testing.T) {
	cases := map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"no principal": {context.Background(), codes.Unauthenticated},
		"anonymous":    {asAnonymous(), codes.Unauthenticated},
		"service":      {asService("svc_order"), codes.PermissionDenied},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			seed(e.inbox, "u1", "n1")
			base := e.inbox.calls
			_, err := e.h.ListNotifications(c.ctx, &notificationv1.ListNotificationsRequest{})
			wantCode(t, "List", err, c.want)
			_, err = e.h.MarkAsRead(c.ctx, &notificationv1.MarkAsReadRequest{Id: "n1"})
			wantCode(t, "MarkAsRead", err, c.want)
			_, err = e.h.MarkAsRead(c.ctx, &notificationv1.MarkAsReadRequest{})
			wantCode(t, "MarkAsRead all", err, c.want)
			_, err = e.h.GetUnreadCount(c.ctx, &notificationv1.GetUnreadCountRequest{})
			wantCode(t, "GetUnreadCount", err, c.want)
			_, err = e.h.SubscribeAlert(c.ctx, &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
			wantCode(t, "SubscribeAlert", err, c.want)
			_, err = e.h.UnsubscribeAlert(c.ctx, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: "s"})
			wantCode(t, "UnsubscribeAlert", err, c.want)
			_, err = e.h.ListAlertSubscriptions(c.ctx, &notificationv1.ListAlertSubscriptionsRequest{})
			wantCode(t, "ListAlertSubscriptions", err, c.want)
			_, err = e.h.GetNotificationPrefs(c.ctx, &notificationv1.GetNotificationPrefsRequest{})
			wantCode(t, "GetNotificationPrefs", err, c.want)
			_, err = e.h.UpdateNotificationPrefs(c.ctx, &notificationv1.UpdateNotificationPrefsRequest{Prefs: &notificationv1.NotificationPrefs{}})
			wantCode(t, "UpdateNotificationPrefs", err, c.want)
			if e.inbox.calls != base || e.alerts.count() != 0 || len(e.prefs.byUser) != 0 || e.inbox.read("n1") {
				t.Fatal("a denied call reached the store")
			}
		})
	}
}

// Scenario: The resolver principal cannot use the per-user RPCs
func TestResolverPrincipalDeniedOnPerUserRPCs(t *testing.T) {
	e := newEnv(t)
	seed(e.inbox, "u1", "n1")
	c := dial(t, e.h)
	ctx := wire("x-principal-id", "service-team-notification", "x-principal-type", "service", "x-principal-scopes", "listing.read,identity.read")
	base := e.inbox.calls
	_, err := c.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{})
	wantCode(t, "List", err, codes.PermissionDenied)
	_, err = c.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: "n1"})
	wantCode(t, "MarkAsRead", err, codes.PermissionDenied)
	_, err = c.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{})
	wantCode(t, "MarkAsRead all", err, codes.PermissionDenied)
	if e.inbox.calls != base || e.inbox.read("n1") {
		t.Fatal("resolver principal reached the store")
	}
}

// Scenario: Notification RPCs serve only the calling user's data (page size clamp)
func TestListNotificationsPageSizeClamp(t *testing.T) {
	cases := []struct {
		name         string
		size, number int32
		wantLimit    int
		wantOffset   int
	}{
		{"zero size defaults to 20", 0, 0, 20, 0},
		{"negative size defaults to 20", -5, 1, 20, 0},
		{"explicit size kept", 50, 1, 50, 0},
		{"exactly 100 kept", 100, 1, 100, 0},
		{"101 clamps to 100", 101, 1, 100, 0},
		{"huge clamps to 100", 100000, 1, 100, 0},
		{"page 3 of 10", 10, 3, 10, 20},
		{"page 0 is page 1", 10, 0, 10, 0},
		{"offset uses the clamped size", 500, 2, 100, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			if _, err := e.h.ListNotifications(asUser("u1"), &notificationv1.ListNotificationsRequest{PageSize: c.size, PageNumber: c.number}); err != nil {
				t.Fatal(err)
			}
			if e.inbox.lastLimit != c.wantLimit || e.inbox.lastOff != c.wantOffset {
				t.Fatalf("limit=%d offset=%d, want %d and %d", e.inbox.lastLimit, e.inbox.lastOff, c.wantLimit, c.wantOffset)
			}
		})
	}
}

// Scenario: Notification RPCs serve only the calling user's data (store errors stay internal)
func TestMapErrsHideInternalCause(t *testing.T) {
	cause := errors.New("pq: connection refused 10.0.0.5")
	check := func(label string, err error) {
		t.Helper()
		st, _ := status.FromError(err)
		if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(err.Error(), "10.0.0.5") {
			t.Errorf("%s: got %v %q", label, st.Code(), st.Message())
		}
	}
	e := newEnv(t)
	e.inbox.err = cause
	ctx := asUser("u1")
	_, err := e.h.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{})
	check("list", err)
	_, err = e.h.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: "x"})
	check("mark", err)
	_, err = e.h.GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{})
	check("unread", err)

	e.alerts.createErr = cause
	_, err = e.h.SubscribeAlert(ctx, &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	check("subscribe", err)

	e.prefs.upsertErr = cause
	_, err = e.h.UpdateNotificationPrefs(ctx, &notificationv1.UpdateNotificationPrefsRequest{Prefs: &notificationv1.NotificationPrefs{}})
	check("prefs", err)
}

// Scenario: Alert and preference RPCs validate input and are always served (unique violation)
func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	e := newEnv(t)
	e.alerts.createErr = &pgconn.PgError{Code: "23505", ConstraintName: "alert_subscriptions_pkey", Message: "duplicate key value"}
	_, err := e.h.SubscribeAlert(asUser("u1"), &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	wantCode(t, "subscribe", err, codes.AlreadyExists)
	if strings.Contains(status.Convert(err).Message(), "alert_subscriptions_pkey") {
		t.Fatalf("constraint name leaked: %q", status.Convert(err).Message())
	}
}

// Scenario: Subscribing twice returns the same subscription
func TestSubscribeAlertIsIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := asUser("u1")
	req := &notificationv1.SubscribeAlertRequest{ListingId: "lst_1", Type: notificationv1.AlertType_ALERT_TYPE_BACK_IN_STOCK}
	r1, err := e.h.SubscribeAlert(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := e.h.SubscribeAlert(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Subscription.Id == "" || r1.Subscription.Id != r2.Subscription.Id {
		t.Fatalf("ids %q vs %q", r1.Subscription.Id, r2.Subscription.Id)
	}
	l, _ := e.h.ListAlertSubscriptions(ctx, &notificationv1.ListAlertSubscriptionsRequest{})
	if len(l.Subscriptions) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(l.Subscriptions))
	}
}

// Scenario: An invalid subscription is rejected
func TestSubscribeAlertValidationIsInvalidArgument(t *testing.T) {
	e := newEnv(t)
	ctx := asUser("u1")
	_, err := e.h.SubscribeAlert(ctx, &notificationv1.SubscribeAlertRequest{ListingId: "", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	wantCode(t, "empty listing", err, codes.InvalidArgument)
	_, err = e.h.SubscribeAlert(ctx, &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_UNSPECIFIED})
	wantCode(t, "unspecified type", err, codes.InvalidArgument)
	_, err = e.h.UnsubscribeAlert(ctx, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: ""})
	wantCode(t, "empty subscription id", err, codes.InvalidArgument)
	_, err = e.h.UpdateNotificationPrefs(ctx, &notificationv1.UpdateNotificationPrefsRequest{})
	wantCode(t, "nil prefs", err, codes.InvalidArgument)
	if e.alerts.count() != 0 || len(e.prefs.byUser) != 0 {
		t.Fatal("a rejected call stored a row")
	}
	p, err := e.h.GetNotificationPrefs(ctx, &notificationv1.GetNotificationPrefsRequest{})
	if err != nil || p.Prefs.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
		t.Fatalf("defaults after rejection: %v %v", p, err)
	}
}

// Scenario: Unsubscribing another user's subscription changes nothing
func TestUnsubscribeForeignSubscriptionIsNoOp(t *testing.T) {
	e := newEnv(t)
	sub, err := e.h.SubscribeAlert(asUser("u2"), &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.h.UnsubscribeAlert(asUser("u1"), &notificationv1.UnsubscribeAlertRequest{SubscriptionId: sub.Subscription.Id}); err != nil {
		t.Fatalf("foreign unsubscribe: %v", err)
	}
	if _, err := e.h.UnsubscribeAlert(asUser("u1"), &notificationv1.UnsubscribeAlertRequest{SubscriptionId: "does-not-exist"}); err != nil {
		t.Fatalf("missing unsubscribe: %v", err)
	}
	l, _ := e.h.ListAlertSubscriptions(asUser("u2"), &notificationv1.ListAlertSubscriptionsRequest{})
	if len(l.Subscriptions) != 1 {
		t.Fatalf("u2 subscriptions = %d, want 1", len(l.Subscriptions))
	}
}

// Scenario: Alert and preference RPCs validate input and are always served (no Unimplemented, panic recovery)
func TestPanicReturnsInternalAndServerKeepsServing(t *testing.T) {
	e := newEnv(t)
	seed(e.inbox, "u1", "n1")
	c := dial(t, e.h)
	e.inbox.panicNext = true
	_, err := c.ListNotifications(wireUser("u1"), &notificationv1.ListNotificationsRequest{})
	wantCode(t, "panicking call", err, codes.Internal)
	if strings.Contains(status.Convert(err).Message(), "boom") {
		t.Fatalf("panic text leaked: %q", status.Convert(err).Message())
	}
	res, err := c.ListNotifications(wireUser("u1"), &notificationv1.ListNotificationsRequest{})
	if err != nil || len(res.Notifications) != 1 {
		t.Fatalf("server stopped serving after panic: %v %v", res, err)
	}
}

// Scenario: Alert and preference RPCs validate input and are always served
func TestEveryRPCIsServedOverTheWire(t *testing.T) {
	e := newEnv(t)
	c := dial(t, e.h)
	ctx := wireUser("u1")
	calls := map[string]func() error{
		"ListNotifications": func() error {
			_, err := c.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{})
			return err
		},
		"MarkAsRead":     func() error { _, err := c.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{}); return err },
		"GetUnreadCount": func() error { _, err := c.GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{}); return err },
		"ListAlertSubscriptions": func() error {
			_, err := c.ListAlertSubscriptions(ctx, &notificationv1.ListAlertSubscriptionsRequest{})
			return err
		},
		"GetNotificationPrefs": func() error {
			_, err := c.GetNotificationPrefs(ctx, &notificationv1.GetNotificationPrefsRequest{})
			return err
		},
		"UpdateNotificationPrefs": func() error {
			_, err := c.UpdateNotificationPrefs(ctx, &notificationv1.UpdateNotificationPrefsRequest{Prefs: &notificationv1.NotificationPrefs{}})
			return err
		},
		"SubscribeAlert": func() error {
			_, err := c.SubscribeAlert(ctx, &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
			return err
		},
		"UnsubscribeAlert": func() error {
			_, err := c.UnsubscribeAlert(ctx, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: "s"})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err != nil {
			t.Errorf("%s: %v", name, fmt.Sprint(err))
		}
	}
}
