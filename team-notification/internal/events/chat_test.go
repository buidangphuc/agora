package events_test

import (
	"strings"
	"testing"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/events"
)

const chatMessageType = "platform.chat.v1.ChatMessage"

func sellerReply() *chatv1.ChatMessage {
	return &chatv1.ChatMessage{
		Id: "m1", ThreadId: "thread-1", SenderId: "seller-1", SenderName: "User sell01",
		RecipientId: "buyer-1", SellerId: "seller-1", Content: "Dạ còn hàng bạn nhé",
	}
}

func chatEnvelope(t *testing.T, eventID string, m *chatv1.ChatMessage) []byte {
	t.Helper()
	return envelopeOf(t, eventID, chatMessageType, m)
}

func TestChat_NotifiesRecipientOnly(t *testing.T) {
	pool := newDB(t)
	h := events.NewChatHandler(pool, nil, nil)

	mustDeliver(t, h, chatEnvelope(t, "evt-1", sellerReply()))

	if got := totalNotifications(t, pool); got != 1 {
		t.Fatalf("want 1 notification, got %d", got)
	}
	n := one(t, pool, "buyer-1")
	if n.GetType() != notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT {
		t.Errorf("type = %v, want CHAT", n.GetType())
	}
	if n.GetLinkUrl() != "/chat/thread-1" {
		t.Errorf("link = %q", n.GetLinkUrl())
	}
	if n.GetTitle() != "Tin nhắn mới từ Người dùng" {
		t.Errorf("title = %q", n.GetTitle())
	}
	if n.GetBody() != "Dạ còn hàng bạn nhé" {
		t.Errorf("body = %q", n.GetBody())
	}
	if got := len(notificationsOf(t, pool, "seller-1")); got != 0 {
		t.Errorf("the sender must not be notified, got %d", got)
	}
}

func TestChat_SenderIsNotNotified(t *testing.T) {
	pool := newDB(t)
	h := events.NewChatHandler(pool, nil, nil)

	m := sellerReply()
	m.RecipientId = m.SenderId
	mustDeliver(t, h, chatEnvelope(t, "evt-self", m))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("sender must not be notified of their own message, got %d", got)
	}
}

func TestChat_MissingRecipient_Skipped(t *testing.T) {
	pool := newDB(t)
	h := events.NewChatHandler(pool, nil, nil)

	m := sellerReply()
	m.RecipientId = ""
	mustDeliver(t, h, chatEnvelope(t, "evt-legacy", m))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("want 0 notifications, got %d", got)
	}
}

func TestChat_DisabledPreferenceSkips(t *testing.T) {
	pool := newDB(t)
	disable(t, pool, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_CHAT)
	h := events.NewChatHandler(pool, nil, nil)

	mustDeliver(t, h, chatEnvelope(t, "evt-pref", sellerReply()))

	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("disabled CHAT pref must skip, got %d", got)
	}
}

func TestChat_OtherTypeDisabledStillNotifies(t *testing.T) {
	pool := newDB(t)
	disable(t, pool, "buyer-1", notificationv1.NotificationType_NOTIFICATION_TYPE_ORDER)
	h := events.NewChatHandler(pool, nil, nil)

	mustDeliver(t, h, chatEnvelope(t, "evt-other", sellerReply()))

	one(t, pool, "buyer-1")
}

func TestChat_BodyTruncatedToRunes(t *testing.T) {
	pool := newDB(t)
	h := events.NewChatHandler(pool, nil, nil)

	m := sellerReply()
	m.Content = strings.Repeat("ả", 300)
	mustDeliver(t, h, chatEnvelope(t, "evt-long", m))

	if got := len([]rune(one(t, pool, "buyer-1").GetBody())); got != 120 {
		t.Fatalf("body runes = %d, want 120", got)
	}
}

func TestChat_IgnoresOtherTypesAndPoison(t *testing.T) {
	pool := newDB(t)
	h := events.NewChatHandler(pool, nil, nil)

	mustDeliver(t, h, envelope(t, "e", "platform.other.v1.X", nil))
	if got := totalNotifications(t, pool); got != 0 {
		t.Fatalf("other type created %d notifications", got)
	}
	requirePermanent(t, deliver(h, []byte("not a proto \xff\xff")))
	requirePermanent(t, deliver(h, envelope(t, "", chatMessageType, nil)))
	requirePermanent(t, deliver(h, envelope(t, "e-bad", chatMessageType, []byte{0xff, 0xff, 0xff})))
}

func TestChat_TitleUsesResolvedName(t *testing.T) {
	pool := newDB(t)
	names := &fakeNames{name: "Nhà Sách An Nhiên"}
	h := events.NewChatHandler(pool, names, nil)

	mustDeliver(t, h, chatEnvelope(t, "evt-1", sellerReply()))

	if got := one(t, pool, "buyer-1").GetTitle(); got != "Tin nhắn mới từ Nhà Sách An Nhiên" {
		t.Fatalf("title = %q", got)
	}
	if len(names.calls) != 1 || names.calls[0] != [2]string{"seller-1", "seller-1"} {
		t.Fatalf("resolver calls = %v", names.calls)
	}
}

func TestChat_LookupFailureStillNotifies(t *testing.T) {
	for _, name := range []string{"", "   "} {
		pool := newDB(t)
		h := events.NewChatHandler(pool, &fakeNames{name: name}, nil)
		mustDeliver(t, h, chatEnvelope(t, "evt-1", sellerReply()))
		if got := one(t, pool, "buyer-1").GetTitle(); got != "Tin nhắn mới từ Người dùng" {
			t.Fatalf("name %q: title = %q", name, got)
		}
	}
}

func TestChat_RedeliveryAfterRestartNotifiesOnce(t *testing.T) {
	pool := newDB(t)
	raw := chatEnvelope(t, "evt-restart", sellerReply())

	mustDeliver(t, events.NewChatHandler(pool, nil, nil), raw)
	mustDeliver(t, events.NewChatHandler(pool, nil, nil), raw)

	one(t, pool, "buyer-1")
}
