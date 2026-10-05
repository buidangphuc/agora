package handler_test

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/handler"
	"github.com/buidangphuc/team-notification/internal/interceptor"
	"github.com/buidangphuc/team-notification/internal/repository"
	"github.com/buidangphuc/team-notification/internal/service"
)

// memRepo is an in-memory NotificationRepository with the same ownership
// semantics as the Postgres one (MarkAsRead is scoped to the owner).
type memRepo struct {
	mu   sync.Mutex
	rows []*notificationv1.Notification
}

func (m *memRepo) CreateNotification(_ context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, n)
	return n, nil
}

func (m *memRepo) ListNotifications(_ context.Context, userID string, _, _ int) ([]*notificationv1.Notification, int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*notificationv1.Notification
	var unread int32
	for _, n := range m.rows {
		if n.UserId == userID {
			out = append(out, n)
			if !n.IsRead {
				unread++
			}
		}
	}
	return out, unread, nil
}

func (m *memRepo) MarkAsRead(_ context.Context, id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		for _, n := range m.rows {
			if n.UserId == userID {
				n.IsRead = true
			}
		}
		return nil
	}
	for _, n := range m.rows {
		if n.Id == id && n.UserId == userID {
			n.IsRead = true
			return nil
		}
	}
	return repository.ErrNotFound
}

func (m *memRepo) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	_, unread, err := m.ListNotifications(ctx, userID, 0, 0)
	return unread, err
}

// asPrincipal runs the real principal interceptor over incoming gateway
// metadata so the handler sees exactly what it would behind the gateway.
func asPrincipal(t *testing.T, id, ptype string) context.Context {
	t.Helper()
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", ptype)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var out context.Context
	_, err := interceptor.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, func(c context.Context, _ any) (any, error) {
		out = c
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	return out
}

func newHandler(repo *memRepo) *handler.NotificationHandler {
	return handler.NewNotificationHandler(repo,
		handler.WithAlertService(service.NewAlertService(repository.NewInMemoryAlertSubscriptionRepo())),
		handler.WithPrefsService(service.NewPrefsService(repository.NewInMemoryNotificationPrefsRepo())),
	)
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (err=%v), want %v", got, err, want)
	}
}

func TestUsersDoNotSeeEachOthersData(t *testing.T) {
	repo := &memRepo{}
	h := newHandler(repo)
	alice := asPrincipal(t, "user_alice", "user")
	bob := asPrincipal(t, "user_bob", "user")

	_, _ = repo.CreateNotification(alice, &notificationv1.Notification{Id: "n_alice", UserId: "user_alice", Title: "for alice"})
	_, _ = repo.CreateNotification(bob, &notificationv1.Notification{Id: "n_bob", UserId: "user_bob", Title: "for bob"})

	t.Run("inbox and unread count", func(t *testing.T) {
		res, err := h.ListNotifications(alice, &notificationv1.ListNotificationsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Notifications) != 1 || res.Notifications[0].Id != "n_alice" {
			t.Fatalf("alice sees %v", res.Notifications)
		}
		resB, _ := h.ListNotifications(bob, &notificationv1.ListNotificationsRequest{})
		if len(resB.Notifications) != 1 || resB.Notifications[0].Id != "n_bob" {
			t.Fatalf("bob sees %v", resB.Notifications)
		}
		cnt, err := h.GetUnreadCount(alice, &notificationv1.GetUnreadCountRequest{})
		if err != nil || cnt.UnreadCount != 1 {
			t.Fatalf("alice unread = %v err=%v", cnt.GetUnreadCount(), err)
		}
	})

	t.Run("alert subscriptions", func(t *testing.T) {
		if _, err := h.SubscribeAlert(alice, &notificationv1.SubscribeAlertRequest{
			ListingId: "lst_1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP,
		}); err != nil {
			t.Fatal(err)
		}
		a, _ := h.ListAlertSubscriptions(alice, &notificationv1.ListAlertSubscriptionsRequest{})
		b, _ := h.ListAlertSubscriptions(bob, &notificationv1.ListAlertSubscriptionsRequest{})
		if len(a.Subscriptions) != 1 || len(b.Subscriptions) != 0 {
			t.Fatalf("alice=%d bob=%d subscriptions", len(a.Subscriptions), len(b.Subscriptions))
		}
		// Bob cannot remove Alice's subscription (scoped delete is a no-op).
		if _, err := h.UnsubscribeAlert(bob, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: a.Subscriptions[0].Id}); err != nil {
			t.Fatal(err)
		}
		a, _ = h.ListAlertSubscriptions(alice, &notificationv1.ListAlertSubscriptionsRequest{})
		if len(a.Subscriptions) != 1 {
			t.Fatalf("bob removed alice's subscription")
		}
	})

	t.Run("prefs", func(t *testing.T) {
		_, err := h.UpdateNotificationPrefs(alice, &notificationv1.UpdateNotificationPrefsRequest{
			Prefs: &notificationv1.NotificationPrefs{DigestFreq: notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY},
		})
		if err != nil {
			t.Fatal(err)
		}
		a, _ := h.GetNotificationPrefs(alice, &notificationv1.GetNotificationPrefsRequest{})
		b, _ := h.GetNotificationPrefs(bob, &notificationv1.GetNotificationPrefsRequest{})
		if a.Prefs.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_WEEKLY {
			t.Fatalf("alice digest = %v", a.Prefs.DigestFreq)
		}
		if b.Prefs.DigestFreq != notificationv1.DigestFrequency_DIGEST_FREQUENCY_OFF {
			t.Fatalf("bob inherited alice's digest: %v", b.Prefs.DigestFreq)
		}
	})
}

func TestMarkAsReadOtherUsersNotificationIsNotFound(t *testing.T) {
	repo := &memRepo{}
	h := newHandler(repo)
	alice := asPrincipal(t, "user_alice", "user")
	bob := asPrincipal(t, "user_bob", "user")
	_, _ = repo.CreateNotification(alice, &notificationv1.Notification{Id: "n_alice", UserId: "user_alice"})

	_, err := h.MarkAsRead(bob, &notificationv1.MarkAsReadRequest{Id: "n_alice"})
	wantCode(t, err, codes.NotFound)
	if repo.rows[0].IsRead {
		t.Fatal("bob marked alice's notification read")
	}
	if _, err := h.MarkAsRead(alice, &notificationv1.MarkAsReadRequest{Id: "n_alice"}); err != nil {
		t.Fatalf("owner mark: %v", err)
	}
	if !repo.rows[0].IsRead {
		t.Fatal("owner mark did not take effect")
	}
}

func TestPerUserRPCsRequireAUserPrincipal(t *testing.T) {
	h := newHandler(&memRepo{})
	cases := map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"no principal": {context.Background(), codes.Unauthenticated},
		"anonymous":    {asPrincipal(t, "anonymous", "anonymous"), codes.Unauthenticated},
		"service":      {asPrincipal(t, "svc_order", "service"), codes.PermissionDenied},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.ListNotifications(c.ctx, &notificationv1.ListNotificationsRequest{})
			wantCode(t, err, c.want)
			_, err = h.MarkAsRead(c.ctx, &notificationv1.MarkAsReadRequest{Id: "x"})
			wantCode(t, err, c.want)
			_, err = h.GetUnreadCount(c.ctx, &notificationv1.GetUnreadCountRequest{})
			wantCode(t, err, c.want)
			_, err = h.SubscribeAlert(c.ctx, &notificationv1.SubscribeAlertRequest{ListingId: "l", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
			wantCode(t, err, c.want)
			_, err = h.UnsubscribeAlert(c.ctx, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: "s"})
			wantCode(t, err, c.want)
			_, err = h.ListAlertSubscriptions(c.ctx, &notificationv1.ListAlertSubscriptionsRequest{})
			wantCode(t, err, c.want)
			_, err = h.GetNotificationPrefs(c.ctx, &notificationv1.GetNotificationPrefsRequest{})
			wantCode(t, err, c.want)
			_, err = h.UpdateNotificationPrefs(c.ctx, &notificationv1.UpdateNotificationPrefsRequest{Prefs: &notificationv1.NotificationPrefs{}})
			wantCode(t, err, c.want)
		})
	}
}
