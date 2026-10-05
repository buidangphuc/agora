package consumer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	orderv1 "github.com/buidangphuc/team-notification/generated/platform/order/v1"
	"github.com/buidangphuc/team-notification/internal/repository"
	"github.com/buidangphuc/team-notification/internal/service"
)

func newPrefs() (*service.PrefsService, *repository.InMemoryNotificationPrefsRepo) {
	repo := repository.NewInMemoryNotificationPrefsRepo()
	return service.NewPrefsService(repo), repo
}

func disable(t *testing.T, repo *repository.InMemoryNotificationPrefsRepo, user string, typ notificationv1.NotificationType) {
	t.Helper()
	_, err := repo.Upsert(context.Background(), user, &notificationv1.NotificationPrefs{
		TypeEnabled: map[string]bool{typ.String(): false},
	})
	if err != nil {
		t.Fatalf("upsert prefs: %v", err)
	}
}

func chatEnvelope(t *testing.T, eventID string, m *chatv1.ChatMessage) []byte {
	t.Helper()
	payload, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return envelope(t, eventID, chatMessageType, payload)
}

func shippedEnvelope(t *testing.T, eventID string, e *orderv1.OrderShipped) []byte {
	t.Helper()
	payload, err := proto.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return envelope(t, eventID, orderShippedType, payload)
}

func sellerReply() *chatv1.ChatMessage {
	return &chatv1.ChatMessage{
		Id: "m1", ThreadId: "thread-1", SenderId: "seller-1", SenderName: "User sell01",
		RecipientId: "buyer-1", Content: "Dạ còn hàng bạn nhé",
	}
}

// ── chat ──

func TestChat_NotifiesRecipientOnly(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-1", sellerReply())); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 1 {
		t.Fatalf("want 1 notification, got %d", notif.count())
	}
	n := notif.created[0]
	if n.GetUserId() != "buyer-1" {
		t.Errorf("notified %q, want the recipient buyer-1 (not the sender)", n.GetUserId())
	}
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT {
		t.Errorf("type = %v, want CHAT", n.GetType())
	}
	if n.GetLinkUrl() != "/chat/thread-1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetTitle() != "Tin nhắn mới từ User sell01" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetBody() != "Dạ còn hàng bạn nhé" {
		t.Errorf("body = %q", n.GetBody())
	}
}

func TestChat_SenderIsNotNotified(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	m := sellerReply()
	m.RecipientId = m.SenderId // degenerate event: recipient == sender
	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-self", m)); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 0 {
		t.Fatalf("sender must not be notified of their own message, got %d", notif.count())
	}
}

func TestChat_MissingRecipient_Skipped(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	m := sellerReply()
	m.RecipientId = ""
	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-legacy", m)); err != nil {
		t.Fatalf("a legacy event without recipient must be a no-op, got %v", err)
	}
	if notif.count() != 0 {
		t.Fatalf("want 0 notifications, got %d", notif.count())
	}
}

func TestChat_RedeliveryNotifiesOnce(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	raw := chatEnvelope(t, "evt-dup", sellerReply())
	for i := 0; i < 3; i++ {
		if err := c.HandleRaw(context.Background(), raw); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	if notif.count() != 1 {
		t.Fatalf("redelivery must notify once, got %d", notif.count())
	}
}

func TestChat_DisabledPreferenceSkips(t *testing.T) {
	notif := &fakeNotif{}
	prefs, repo := newPrefs()
	disable(t, repo, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT)
	c := NewChatConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-pref", sellerReply())); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 0 {
		t.Fatalf("disabled CHAT pref must skip, got %d", notif.count())
	}
}

func TestChat_OtherTypeDisabledStillNotifies(t *testing.T) {
	notif := &fakeNotif{}
	prefs, repo := newPrefs()
	disable(t, repo, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	c := NewChatConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-other", sellerReply())); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 1 {
		t.Fatalf("an unrelated disabled type must not block CHAT, got %d", notif.count())
	}
}

func TestChat_BodyTruncatedToRunes(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	m := sellerReply()
	m.Content = strings.Repeat("ả", 300)
	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-long", m)); err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(notif.created[0].GetBody())); got != 120 {
		t.Fatalf("body runes = %d, want 120", got)
	}
}

func TestChat_CreateFailureIsRetriedNotDeduped(t *testing.T) {
	notif := &fakeNotif{err: errors.New("db down")}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)
	raw := chatEnvelope(t, "evt-retry", sellerReply())

	if err := c.HandleRaw(context.Background(), raw); err == nil || errors.Is(err, ErrPermanent) {
		t.Fatalf("want a transient error, got %v", err)
	}
	notif.err = nil
	if err := c.HandleRaw(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("redelivery after a failure must create it, got %d", notif.count())
	}
}

func TestChat_IgnoresOtherTypesAndPoison(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), envelope(t, "e", "platform.other.v1.X", nil)); err != nil || notif.count() != 0 {
		t.Fatalf("other type: err=%v count=%d", err, notif.count())
	}
	if err := c.HandleRaw(context.Background(), []byte("not a proto \xff\xff")); !errors.Is(err, ErrPermanent) {
		t.Fatalf("malformed envelope must be permanent, got %v", err)
	}
	if err := c.HandleRaw(context.Background(), envelope(t, "", chatMessageType, nil)); !errors.Is(err, ErrPermanent) {
		t.Fatalf("missing event_id must be permanent, got %v", err)
	}
}

// ── order ──

func shipped() *orderv1.OrderShipped {
	return &orderv1.OrderShipped{
		OrderId: "ord-1", BuyerId: "buyer-1", SellerId: "seller-1",
		Carrier: "GHN", TrackingCode: "GHN-VN-123", ShippedAt: timestamppb.Now(),
	}
}

func TestOrderShipped_NotifiesBuyer(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewOrderConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), shippedEnvelope(t, "evt-s1", shipped())); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 1 {
		t.Fatalf("want 1 notification, got %d", notif.count())
	}
	n := notif.created[0]
	if n.GetUserId() != "buyer-1" || n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER {
		t.Errorf("user/type = %q/%v", n.GetUserId(), n.GetType())
	}
	if n.GetLinkUrl() != "/account/orders/ord-1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetTitle() != "Đơn hàng đã được giao cho GHN" || n.GetBody() != "Mã vận đơn: GHN-VN-123" {
		t.Errorf("title/body = %q / %q", n.GetTitle(), n.GetBody())
	}
}

func TestOrderPaidEvent_IsIgnored(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewOrderConsumer(notif, prefs, nil)

	payload, err := proto.Marshal(&orderv1.OrderPaidEvent{OrderId: "ord-1", BuyerId: "buyer-1"})
	if err != nil {
		t.Fatal(err)
	}
	raw := envelope(t, "evt-paid", "platform.order.v1.OrderPaidEvent", payload)
	if err := c.HandleRaw(context.Background(), raw); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 0 {
		t.Fatalf("OrderPaidEvent must be ignored, got %d notifications", notif.count())
	}
}

func TestOrderShipped_RedeliveryNotifiesOnce(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewOrderConsumer(notif, prefs, nil)

	raw := shippedEnvelope(t, "evt-s-dup", shipped())
	for i := 0; i < 3; i++ {
		if err := c.HandleRaw(context.Background(), raw); err != nil {
			t.Fatal(err)
		}
	}
	if notif.count() != 1 {
		t.Fatalf("redelivery must notify once, got %d", notif.count())
	}
}

func TestOrderShipped_DisabledPreferenceSkips(t *testing.T) {
	notif := &fakeNotif{}
	prefs, repo := newPrefs()
	disable(t, repo, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	c := NewOrderConsumer(notif, prefs, nil)

	if err := c.HandleRaw(context.Background(), shippedEnvelope(t, "evt-s-pref", shipped())); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 0 {
		t.Fatalf("disabled ORDER pref must skip, got %d", notif.count())
	}
}

func TestOrderShipped_MissingBuyerIsPermanent(t *testing.T) {
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewOrderConsumer(notif, prefs, nil)

	e := shipped()
	e.BuyerId = ""
	if err := c.HandleRaw(context.Background(), shippedEnvelope(t, "evt-s-bad", e)); !errors.Is(err, ErrPermanent) {
		t.Fatalf("want permanent error, got %v", err)
	}
}
