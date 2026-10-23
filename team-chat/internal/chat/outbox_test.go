package chat_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/store"
)

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func okBuilder(_ context.Context, m store.ChatMessage) (outbox.Row, error) {
	return outbox.Row{EventID: "evt-" + m.ID, AggregateType: "ChatThread", AggregateID: m.ThreadID, EventType: "T", Payload: []byte("x")}, nil
}

func TestDeniedSenderEnqueuesNoEvent(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(okBuilder)
	svc := chat.NewService(repo, quiet())
	th, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "l1", "t", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(ctx, th.ID, "intruder-99", "x", "hey"); !errors.Is(err, chat.ErrUnauthorizedChat) {
		t.Fatalf("err = %v, want ErrUnauthorizedChat", err)
	}
	if _, err := svc.SendMessage(ctx, "no-such-thread", "buyer-1", "b", "hey"); !errors.Is(err, store.ErrThreadNotFound) {
		t.Fatalf("err = %v, want ErrThreadNotFound", err)
	}
	if rows := repo.outboxRows(); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(rows))
	}
	if _, err := svc.SendMessage(ctx, th.ID, "buyer-1", "b", "hello"); err != nil {
		t.Fatal(err)
	}
	if rows := repo.outboxRows(); len(rows) != 1 || rows[0].AggregateID != th.ID {
		t.Fatalf("allowed send rows = %+v, want exactly one for the thread", rows)
	}
}

func TestSendMessagePassesRecipientAndSellerToOutbox(t *testing.T) {
	ctx := context.Background()
	var got []string
	repo := newFakeRepo(func(_ context.Context, m store.ChatMessage) (outbox.Row, error) {
		got = append(got, m.SenderID+">"+m.RecipientID+"@"+m.SellerID)
		return okBuilder(ctx, m)
	})
	svc := chat.NewService(repo, quiet())
	th, _ := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "l1", "t", "")
	_, _ = svc.SendMessage(ctx, th.ID, "buyer-1", "b", "q")
	_, _ = svc.SendMessage(ctx, th.ID, "seller-1", "s", "a")
	if len(got) != 2 || got[0] != "buyer-1>seller-1@seller-1" || got[1] != "seller-1>buyer-1@seller-1" {
		t.Fatalf("sender>recipient@seller = %v", got)
	}
}

func TestSendMessageOutboxFailureStoresNothing(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(func(context.Context, store.ChatMessage) (outbox.Row, error) {
		return outbox.Row{}, errors.New("boom")
	})
	svc := chat.NewService(repo, quiet())
	th, _ := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "l1", "t", "")
	if _, err := svc.SendMessage(ctx, th.ID, "buyer-1", "b", "hello"); err == nil {
		t.Fatal("expected the send to fail")
	}
	msgs, total, _ := svc.GetThreadMessages(ctx, th.ID, "buyer-1", 1, 50)
	if len(msgs) != 0 || total != 0 || len(repo.outboxRows()) != 0 {
		t.Fatalf("stored message=%d total=%d outbox=%d, want all 0", len(msgs), total, len(repo.outboxRows()))
	}
}

func TestSearchMessagesBlankQueryIsEmpty(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo(nil)
	svc := chat.NewService(repo, quiet())
	th, _ := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "l1", "t", "")
	_, _ = svc.SendMessage(ctx, th.ID, "buyer-1", "b", "needle")
	for _, q := range []string{"   ", "", "\t\n"} {
		msgs, total, err := svc.SearchMessages(ctx, "buyer-1", q, 1, 20)
		if err != nil || len(msgs) != 0 || total != 0 {
			t.Fatalf("query %q: msgs=%d total=%d err=%v, want empty", q, len(msgs), total, err)
		}
	}
	if n := repo.searches(); n != 0 {
		t.Fatalf("repository search calls = %d, want 0", n)
	}
	if msgs, _, _ := svc.SearchMessages(ctx, "buyer-1", " needle ", 1, 20); len(msgs) != 1 {
		t.Fatalf("trimmed query found %d, want 1", len(msgs))
	}
}
