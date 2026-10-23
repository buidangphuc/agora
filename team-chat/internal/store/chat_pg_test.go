package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-chat/internal/store"
	"github.com/buidangphuc/team-chat/migrations"
)

func chatDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := freshSchema(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return openPool(t, url)
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func rowFor(m store.ChatMessage) outbox.Row {
	return outbox.Row{EventID: "evt-" + m.ID, AggregateType: "ChatThread", AggregateID: m.ThreadID, EventType: "T", Payload: []byte(m.RecipientID + "|" + m.SellerID)}
}

func TestOutboxTableNameIsExported(t *testing.T) {
	if store.OutboxTable != "chat_outbox_events" {
		t.Fatalf("OutboxTable = %q", store.OutboxTable)
	}
}

func TestSaveMessageIsAtomicWithOutbox(t *testing.T) {
	pool := chatDB(t)
	ctx := context.Background()
	repo := store.NewPostgresChatRepository(pool, store.WithMessageOutbox(func(_ context.Context, m store.ChatMessage) (outbox.Row, error) {
		return rowFor(m), nil
	}))
	th, err := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Sản phẩm", "")
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	saved, err := repo.SaveMessage(ctx, store.ChatMessage{ThreadID: th.ID, SenderID: "buyer-1", SenderName: "b", Content: "hi", RecipientID: "seller-1", SellerID: "seller-1"})
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if m, o := countRows(t, pool, "chat_messages"), countRows(t, pool, store.OutboxTable); m != 1 || o != 1 {
		t.Fatalf("messages=%d outbox=%d, want 1/1", m, o)
	}
	var agg, status, payload string
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT aggregate_id, status, payload::text, seq FROM `+store.OutboxTable+` WHERE event_id = $1`, "evt-"+saved.ID).Scan(&agg, &status, &payload, &seq); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if agg != th.ID || status != "pending" || seq <= 0 {
		t.Fatalf("outbox aggregate=%q status=%q seq=%d, want %q pending seq>0", agg, status, seq, th.ID)
	}
	if !strings.Contains(payload, "seller-1") {
		t.Fatalf("payload %q lost the recipient/seller", payload)
	}
	var last string
	var unread int
	if err := pool.QueryRow(ctx, `SELECT last_message_text, unread_count_seller FROM chat_threads WHERE id=$1`, th.ID).Scan(&last, &unread); err != nil || last != "hi" || unread != 1 {
		t.Fatalf("thread last=%q unread=%d err=%v", last, unread, err)
	}
}

func TestSaveMessageRollsBackWhenOutboxFails(t *testing.T) {
	pool := chatDB(t)
	ctx := context.Background()
	seedRepo := store.NewPostgresChatRepository(pool)
	th, err := seedRepo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Sản phẩm", "")
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	msg := store.ChatMessage{ThreadID: th.ID, SenderID: "seller-1", SenderName: "s", Content: "hi", RecipientID: "buyer-1"}

	cases := map[string]func(context.Context, store.ChatMessage) (outbox.Row, error){
		"builder error": func(context.Context, store.ChatMessage) (outbox.Row, error) {
			return outbox.Row{}, errors.New("boom")
		},
		"empty event id": func(_ context.Context, m store.ChatMessage) (outbox.Row, error) {
			r := rowFor(m)
			r.EventID = ""
			return r, nil
		},
		"null payload rejected by the table": func(_ context.Context, m store.ChatMessage) (outbox.Row, error) {
			r := rowFor(m)
			r.Payload = nil
			return r, nil
		},
	}
	for name, b := range cases {
		repo := store.NewPostgresChatRepository(pool, store.WithMessageOutbox(b))
		if _, err := repo.SaveMessage(ctx, msg); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if m, o := countRows(t, pool, "chat_messages"), countRows(t, pool, store.OutboxTable); m != 0 || o != 0 {
			t.Fatalf("%s: messages=%d outbox=%d, want 0/0", name, m, o)
		}
		var unread int
		if err := pool.QueryRow(ctx, `SELECT unread_count_buyer FROM chat_threads WHERE id=$1`, th.ID).Scan(&unread); err != nil || unread != 0 {
			t.Fatalf("%s: thread unread=%d err=%v, want 0 (rolled back)", name, unread, err)
		}
	}
}

func TestSearchMessagesScopedToParticipant(t *testing.T) {
	pool := chatDB(t)
	ctx := context.Background()
	repo := store.NewPostgresChatRepository(pool)
	tA, _ := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Item", "")
	tB, _ := repo.GetOrCreateThread(ctx, "buyer-2", "seller-2", "listing-2", "Item", "")
	if _, err := repo.SaveMessage(ctx, store.ChatMessage{ThreadID: tA.ID, SenderID: "buyer-1", Content: "the NEEDLE here"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveMessage(ctx, store.ChatMessage{ThreadID: tB.ID, SenderID: "buyer-2", Content: "another needle there"}); err != nil {
		t.Fatal(err)
	}
	msgs, total, err := repo.SearchMessages(ctx, "buyer-1", "needle", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(msgs) != 1 || msgs[0].ThreadID != tA.ID {
		t.Fatalf("buyer-1 got total=%d msgs=%+v, want only thread A", total, msgs)
	}
	msgs, total, _ = repo.SearchMessages(ctx, "seller-2", "needle", 0, 10)
	if total != 1 || len(msgs) != 1 || msgs[0].ThreadID != tB.ID {
		t.Fatalf("seller-2 got total=%d msgs=%+v, want only thread B", total, msgs)
	}
	msgs, total, _ = repo.SearchMessages(ctx, "stranger", "needle", 0, 10)
	if total != 0 || len(msgs) != 0 {
		t.Fatalf("stranger got total=%d, want 0", total)
	}
}

func TestUnreadCounts(t *testing.T) {
	pool := chatDB(t)
	ctx := context.Background()
	repo := store.NewPostgresChatRepository(pool)
	th, _ := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Product", "")
	for _, c := range []string{"a", "b"} {
		if _, err := repo.SaveMessage(ctx, store.ChatMessage{ThreadID: th.ID, SenderID: "buyer-1", SenderName: "B", Content: c}); err != nil {
			t.Fatal(err)
		}
	}
	threads, total, err := repo.ListThreadsForUser(ctx, "seller-1", 0, 10)
	if err != nil || total != 1 || len(threads) != 1 {
		t.Fatalf("list: total=%d len=%d err=%v", total, len(threads), err)
	}
	got := threads[0]
	if got.LastMessageText != "b" || got.LastMessageAt == nil || got.UnreadCountSeller != 2 || got.UnreadCountBuyer != 0 {
		t.Fatalf("thread = %+v, want last b, time set, seller unread 2, buyer 0", got)
	}
	if err := repo.MarkThreadRead(ctx, th.ID, "seller-1"); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.GetThreadByID(ctx, th.ID)
	if after.UnreadCountSeller != 0 {
		t.Fatalf("seller unread after read = %d", after.UnreadCountSeller)
	}
	if _, err := repo.SaveMessage(ctx, store.ChatMessage{ThreadID: th.ID, SenderID: "seller-1", Content: "reply"}); err != nil {
		t.Fatal(err)
	}
	after, _ = repo.GetThreadByID(ctx, th.ID)
	if after.UnreadCountBuyer != 1 {
		t.Fatalf("buyer unread = %d, want 1", after.UnreadCountBuyer)
	}
	msgs, total, _ := repo.GetThreadMessages(ctx, th.ID, 0, 10)
	if total != 3 || len(msgs) != 3 || msgs[0].Content != "a" || msgs[2].Content != "reply" {
		t.Fatalf("messages not chronological: total=%d %+v", total, msgs)
	}
	if _, err := repo.GetThreadByID(ctx, "missing"); !errors.Is(err, store.ErrThreadNotFound) {
		t.Fatalf("err = %v, want ErrThreadNotFound", err)
	}
}
