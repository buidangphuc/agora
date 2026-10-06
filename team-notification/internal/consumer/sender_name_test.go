package consumer

import (
	"context"
	"testing"
)

type fakeNames struct {
	name  string
	calls [][2]string
}

func (f *fakeNames) ResolveSenderName(_ context.Context, senderID, sellerID string) string {
	f.calls = append(f.calls, [2]string{senderID, sellerID})
	return f.name
}

func chatTitle(t *testing.T, names SenderNameResolver) string {
	t.Helper()
	notif := &fakeNotif{}
	prefs, _ := newPrefs()
	c := NewChatConsumer(notif, prefs, nil)
	if names != nil {
		c.WithOptions(WithSenderNameResolver(names))
	}
	if err := c.HandleRaw(context.Background(), chatEnvelope(t, "evt-1", sellerReply())); err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if notif.count() != 1 {
		t.Fatalf("want 1 notification, got %d", notif.count())
	}
	return notif.created[0].GetTitle()
}

func TestChat_TitleUsesResolvedName(t *testing.T) {
	names := &fakeNames{name: "Nhà Sách An Nhiên"}
	if got := chatTitle(t, names); got != "Tin nhắn mới từ Nhà Sách An Nhiên" {
		t.Fatalf("title = %q", got)
	}
	// The resolver gets the sender and the thread's seller from the event.
	if len(names.calls) != 1 || names.calls[0] != [2]string{"seller-1", "seller-1"} {
		t.Fatalf("resolver calls = %v", names.calls)
	}
}

// A failed or empty lookup still notifies, with the neutral label.
func TestChat_LookupFailureStillNotifies(t *testing.T) {
	if got := chatTitle(t, &fakeNames{name: ""}); got != "Tin nhắn mới từ Người dùng" {
		t.Fatalf("title = %q", got)
	}
	if got := chatTitle(t, &fakeNames{name: "   "}); got != "Tin nhắn mới từ Người dùng" {
		t.Fatalf("blank name title = %q", got)
	}
}
