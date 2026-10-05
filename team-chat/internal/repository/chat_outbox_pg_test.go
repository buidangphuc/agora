package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func chatPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres chat outbox test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	files, _ := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	sort.Strings(files)
	for _, f := range files {
		ddl, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	for _, q := range []string{`DELETE FROM chat_outbox_events`, `DELETE FROM chat_messages`, `DELETE FROM chat_threads`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// The message and its outbox row are one transaction. A failure after the message
// insert (builder error, or the outbox insert itself failing) stores neither; success
// stores both. Runs only against a disposable Postgres (TEST_DATABASE_URL).
func TestSaveMessageIsAtomicWithOutbox_Postgres(t *testing.T) {
	pool := chatPool(t)
	ctx := context.Background()

	seed, err := NewPostgresChatRepository(pool).GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Sản phẩm", "")
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	msg := ChatMessage{ThreadID: seed.ID, SenderID: "seller-1", SenderName: "s", Content: "hi", RecipientID: "buyer-1"}

	// 1. Builder fails: nothing stored.
	failing := NewPostgresChatRepository(pool, WithMessageOutbox(func(context.Context, ChatMessage) (OutboxRow, error) {
		return OutboxRow{}, errors.New("boom")
	}))
	if _, err := failing.SaveMessage(ctx, msg); err == nil {
		t.Fatal("expected error when the outbox builder fails")
	}
	if m, o := countRows(t, pool, "chat_messages"), countRows(t, pool, "chat_outbox_events"); m != 0 || o != 0 {
		t.Fatalf("after builder failure: messages=%d outbox=%d, want 0/0", m, o)
	}

	// 2. Outbox insert fails after the message insert (NULL payload violates NOT NULL).
	badRow := NewPostgresChatRepository(pool, WithMessageOutbox(func(_ context.Context, m ChatMessage) (OutboxRow, error) {
		return OutboxRow{EventID: "evt-bad", AggregateType: "ChatThread", AggregateID: m.ThreadID, EventType: "T", Payload: nil}, nil
	}))
	if _, err := badRow.SaveMessage(ctx, msg); err == nil {
		t.Fatal("expected error when the outbox insert fails")
	}
	if m, o := countRows(t, pool, "chat_messages"), countRows(t, pool, "chat_outbox_events"); m != 0 || o != 0 {
		t.Fatalf("after outbox insert failure: messages=%d outbox=%d, want 0/0", m, o)
	}
	var unread int
	if err := pool.QueryRow(ctx, `SELECT unread_count_buyer FROM chat_threads WHERE id = $1`, seed.ID).Scan(&unread); err != nil || unread != 0 {
		t.Fatalf("thread unread rolled back? unread=%d err=%v", unread, err)
	}

	// 3. Success: both stored, outbox row keyed by thread id and pending.
	good := NewPostgresChatRepository(pool, WithMessageOutbox(func(_ context.Context, m ChatMessage) (OutboxRow, error) {
		return OutboxRow{EventID: "evt-" + m.ID, AggregateType: "ChatThread", AggregateID: m.ThreadID, EventType: "T", Payload: []byte("x")}, nil
	}))
	saved, err := good.SaveMessage(ctx, msg)
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if m, o := countRows(t, pool, "chat_messages"), countRows(t, pool, "chat_outbox_events"); m != 1 || o != 1 {
		t.Fatalf("after success: messages=%d outbox=%d, want 1/1", m, o)
	}
	var agg, status string
	if err := pool.QueryRow(ctx, `SELECT aggregate_id, status FROM chat_outbox_events WHERE event_id = $1`, "evt-"+saved.ID).Scan(&agg, &status); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if agg != seed.ID || status != "pending" {
		t.Fatalf("outbox row aggregate=%q status=%q, want %q pending", agg, status, seed.ID)
	}
}
