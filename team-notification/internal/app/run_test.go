package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/app"
)

func chatEvent(t *testing.T, eventID string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&chatv1.ChatMessage{
		Id: "m1", ThreadId: "thread-1", SenderId: "seller-1", RecipientId: "buyer-1",
		SellerId: "seller-1", Content: "Dạ còn hàng bạn nhé",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: "platform.chat.v1.ChatMessage", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: Boot uses the real database and broker or fails
	s := notifSettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		if !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("boot error %q does not name postgres as the failed dependency", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving or hanging (Postgres open must be bounded and fail fast)")
	}
	assertPortFree(t, s.Port)
}

func TestMigrateThenRunServesAllRPCs(t *testing.T) {
	// scenario: Migrate then boot serves every RPC
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	r := start(t, notifSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = asUser(ctx, "buyer-1")
	c := notificationv1.NewNotificationServiceClient(r.conn)

	calls := map[string]func() error{
		"ListNotifications": func() error {
			_, err := c.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{PageSize: 10, PageNumber: 1})
			return err
		},
		"GetUnreadCount": func() error { _, err := c.GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{}); return err },
		"MarkAsRead":     func() error { _, err := c.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{}); return err },
		"ListAlertSubscriptions": func() error {
			_, err := c.ListAlertSubscriptions(ctx, &notificationv1.ListAlertSubscriptionsRequest{})
			return err
		},
		"GetNotificationPrefs": func() error {
			_, err := c.GetNotificationPrefs(ctx, &notificationv1.GetNotificationPrefsRequest{})
			return err
		},
		"UpdateNotificationPrefs": func() error {
			_, err := c.UpdateNotificationPrefs(ctx, &notificationv1.UpdateNotificationPrefsRequest{
				Prefs: &notificationv1.NotificationPrefs{TypeEnabled: map[string]bool{"NOTIFICATION_TYPE_CHAT": false}},
			})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err != nil {
			t.Errorf("%s = %v, want OK", name, err)
		}
	}

	sub, err := c.SubscribeAlert(ctx, &notificationv1.SubscribeAlertRequest{ListingId: "listing-1", Type: notificationv1.AlertType_ALERT_TYPE_PRICE_DROP})
	if err != nil {
		t.Fatalf("SubscribeAlert = %v, want OK", err)
	}
	if _, err := c.UnsubscribeAlert(ctx, &notificationv1.UnsubscribeAlertRequest{SubscriptionId: sub.GetSubscription().GetId()}); err != nil {
		t.Errorf("UnsubscribeAlert = %v, want OK", err)
	}
}

func TestChatEventBecomesNotificationWithin30s(t *testing.T) {
	// scenario: A chat message becomes a notification within 30 seconds
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	s := notifSettings(testkit.FreePort(t), url, brokers)
	testkit.Produce(t, brokers, s.Notification.ChatTopic, "buyer-1", chatEvent(t, "evt-chat-1"))

	r := start(t, s)
	waitServing(t, r)

	c := notificationv1.NewNotificationServiceClient(r.conn)
	var got *notificationv1.Notification
	testkit.Eventually(t, 30*time.Second, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := c.ListNotifications(asUser(ctx, "buyer-1"), &notificationv1.ListNotificationsRequest{PageSize: 10, PageNumber: 1})
		if err != nil || len(res.GetNotifications()) == 0 {
			return false
		}
		got = res.GetNotifications()[0]
		return true
	})
	if got.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT {
		t.Errorf("type = %v, want CHAT", got.GetType())
	}
	if got.GetLinkUrl() != "/chat/thread-1" {
		t.Errorf("link = %q, want /chat/thread-1", got.GetLinkUrl())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	other, err := c.ListNotifications(asUser(ctx, "buyer-2"), &notificationv1.ListNotificationsRequest{PageSize: 10, PageNumber: 1})
	if err != nil || len(other.GetNotifications()) != 0 {
		t.Errorf("another user sees %v (err %v), want none", other.GetNotifications(), err)
	}
}

func TestConsumerErrorStopsRun(t *testing.T) {
	// scenario: Boot uses the real database and broker or fails
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	s := notifSettings(testkit.FreePort(t), url, brokers)
	s.Notification.ChatDLQTopic = "invalid topic name!"
	testkit.Produce(t, brokers, s.Notification.ChatTopic, "buyer-1", []byte("not a protobuf envelope"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil after a consumer failed, want the consumer error")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run returned %v, want a consumer error, not cancellation", err)
		}
		if !strings.Contains(err.Error(), "dead-letter") {
			t.Fatalf("Run error %q does not carry the consumer's dead-letter failure", err)
		}
	case <-time.After(80 * time.Second):
		t.Fatal("Run kept running after a consumer returned an error")
	}
	assertPortFree(t, s.Port)
}

func TestReadinessFollowsPostgresLivenessDoesNot(t *testing.T) {
	// scenario: Readiness follows dependencies and liveness does not
	url := migratedURL(t)
	r := start(t, notifSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while Postgres is down, want SERVING", got)
	}

	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestReadinessIgnoresKafka(t *testing.T) {
	// scenario: A broker outage does not flip readiness
	url := migratedURL(t)
	r := start(t, notifSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	time.Sleep(healthWindow + time.Second)
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v with Kafka unreachable, want SERVING", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := notificationv1.NewNotificationServiceClient(r.conn).GetUnreadCount(asUser(ctx, "buyer-1"), &notificationv1.GetUnreadCountRequest{}); err != nil {
		t.Fatalf("GetUnreadCount with Kafka unreachable = %v, want OK", err)
	}
}

func TestLivenessCheckLatencyUnder5ms(t *testing.T) {
	// scenario: Liveness answers from memory
	url := migratedURL(t)
	r := start(t, notifSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)
	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })

	hc := grpc_health_v1.NewHealthClient(r.conn)
	probe := func() time.Duration {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		begin := time.Now()
		resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"})
		d := time.Since(begin)
		if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("liveness = %v, %v with Postgres down, want SERVING", resp.GetStatus(), err)
		}
		return d
	}
	probe()
	best := time.Hour
	for i := 0; i < 50; i++ {
		if d := probe(); d < best {
			best = d
		}
	}
	if best >= 5*time.Millisecond {
		t.Fatalf("best of 50 liveness checks = %v, want under 5ms (answered from memory)", best)
	}
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Readiness follows dependencies and liveness does not
	url := migratedURL(t)
	s := notifSettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first watch = %v, %v, want SERVING", first.GetStatus(), err)
	}

	r.cancel()
	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("watch ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("watch after cancel = %v, want NOT_SERVING", next.GetStatus())
	}
	select {
	case <-r.done:
		t.Fatal("Run returned before the drain delay elapsed")
	default:
	}
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
	}
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Boot uses the real database and broker or fails
	url := migratedURL(t)
	r := start(t, notifSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err == nil {
		err = stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}})
		if err == nil {
			_, err = stream.Recv()
		} else if _, rerr := stream.Recv(); rerr != nil {
			err = rerr
		}
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}
