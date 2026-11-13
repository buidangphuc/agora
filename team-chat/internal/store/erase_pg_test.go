package store_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-chat/internal/store"
)

const (
	erasedText = "Tin nhắn đã bị xóa"
	erasedName = "Người dùng đã xóa"
	victim     = "user-erase-victim"
	seller     = "user-erase-seller"
	bystander  = "user-erase-bystander"
	bySeller   = "user-erase-bystander-seller"
)

type erasureFixture struct {
	pool *pgxpool.Pool
	repo *store.PostgresChatRepository
	t1   store.ChatThread
	t2   store.ChatThread
	t3   store.ChatThread
	t4   store.ChatThread
}

func send(t *testing.T, repo *store.PostgresChatRepository, th string, sender, name, content string, mt int32, payload string) store.ChatMessage {
	t.Helper()
	m, err := repo.SaveMessage(context.Background(), store.ChatMessage{
		ThreadID: th, SenderID: sender, SenderName: name, Content: content, MessageType: mt, Payload: payload,
	})
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return m
}

func seedErasure(t *testing.T) erasureFixture {
	t.Helper()
	pool := chatDB(t)
	ctx := context.Background()
	repo := store.NewPostgresChatRepository(pool)
	mk := func(buyer, sell, listing string) store.ChatThread {
		th, err := repo.GetOrCreateThread(ctx, buyer, sell, listing, "Item", "")
		if err != nil {
			t.Fatalf("seed thread: %v", err)
		}
		return th
	}
	f := erasureFixture{pool: pool, repo: repo}
	f.t1 = mk(victim, seller, "l1")
	f.t2 = mk(victim, "seller-two", "l2")
	f.t3 = mk("buyer-three", victim, "l3")
	f.t4 = mk(bystander, bySeller, "l4")

	send(t, repo, f.t1.ID, victim, "Victim Name", "secret one", 0, "")
	send(t, repo, f.t1.ID, seller, "Shop", "seller reply 1", 0, "")
	send(t, repo, f.t1.ID, victim, "Victim Name", "secret two", 1, `{"listing":"l1"}`)
	send(t, repo, f.t1.ID, seller, "Shop", "seller reply 2", 0, "")
	send(t, repo, f.t1.ID, victim, "Victim Name", "secret three last", 0, "")

	send(t, repo, f.t2.ID, victim, "Victim Name", "ask price", 0, "")
	send(t, repo, f.t2.ID, "seller-two", "Shop Two", "counterparty last", 0, "")

	send(t, repo, f.t3.ID, "buyer-three", "Buyer Three", "hello shop", 0, "")
	send(t, repo, f.t3.ID, victim, "Victim Shop", "victim seller last", 2, `{"quick":"yes"}`)

	send(t, repo, f.t4.ID, bystander, "By Stander", "bystander text", 1, `{"k":"v"}`)
	send(t, repo, f.t4.ID, bySeller, "By Seller", "bystander seller last", 0, "")
	return f
}

func dump(t *testing.T, pool *pgxpool.Pool, query string, args ...any) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func snapshotAll(t *testing.T, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	return map[string][]string{
		"messages": dump(t, pool, `SELECT m::text FROM chat_messages m ORDER BY id`),
		"threads":  dump(t, pool, `SELECT th::text FROM chat_threads th ORDER BY id`),
	}
}

func messagesOf(t *testing.T, repo *store.PostgresChatRepository, thread string) []store.ChatMessage {
	t.Helper()
	msgs, _, err := repo.GetThreadMessages(context.Background(), thread, 0, 100)
	if err != nil {
		t.Fatalf("GetThreadMessages: %v", err)
	}
	return msgs
}

func eraseOK(t *testing.T, repo *store.PostgresChatRepository, user string) int64 {
	t.Helper()
	n, err := repo.Erase(context.Background(), user)
	if err != nil {
		t.Fatalf("Erase(%s): %v", user, err)
	}
	return n
}

// Scenario: The user's messages are replaced and the thread is kept
func TestEraseReplacesUserMessagesAndKeepsThread(t *testing.T) {
	f := seedErasure(t)
	before := messagesOf(t, f.repo, f.t1.ID)
	if n := eraseOK(t, f.repo, victim); n != 6 {
		t.Fatalf("Erase returned %d, want 6 messages replaced (3+1+2)", n)
	}
	after := messagesOf(t, f.repo, f.t1.ID)
	if len(after) != len(before) || len(after) != 5 {
		t.Fatalf("thread lost messages: before %d after %d", len(before), len(after))
	}
	for i, m := range after {
		if before[i].SenderID == victim {
			if m.Content != erasedText || m.SenderName != erasedName {
				t.Errorf("victim message %d = %q/%q, want %q/%q", i, m.Content, m.SenderName, erasedText, erasedName)
			}
			if m.SenderID != victim {
				t.Errorf("victim message %d lost its sender id %q", i, m.SenderID)
			}
		} else if !reflect.DeepEqual(m, before[i]) {
			t.Errorf("seller message %d changed: %+v -> %+v", i, before[i], m)
		}
	}
	threads, _, err := f.repo.ListThreadsForUser(context.Background(), seller, 0, 10)
	if err != nil || len(threads) != 1 || threads[0].ID != f.t1.ID {
		t.Fatalf("thread no longer lists for the seller: %v %+v", err, threads)
	}
	for _, tbl := range []string{"chat_messages"} {
		if n := countRows(t, f.pool, tbl); n != 11 {
			t.Fatalf("%s rows = %d, want 11 (nothing deleted)", tbl, n)
		}
	}
}

// Scenario: The user's messages are replaced and the thread is kept (the user as seller)
func TestEraseReplacesMessagesSentAsSeller(t *testing.T) {
	f := seedErasure(t)
	eraseOK(t, f.repo, victim)
	msgs := messagesOf(t, f.repo, f.t3.ID)
	if len(msgs) != 2 {
		t.Fatalf("thread t3 has %d messages, want 2", len(msgs))
	}
	for _, m := range msgs {
		if m.SenderID == victim && (m.Content != erasedText || m.SenderName != erasedName) {
			t.Errorf("seller-side message not replaced: %+v", m)
		}
		if m.SenderID == "buyer-three" && (m.Content != "hello shop" || m.SenderName != "Buyer Three") {
			t.Errorf("counterparty message changed: %+v", m)
		}
	}
}

// Scenario: A rich message payload is emptied
func TestEraseEmptiesRichPayload(t *testing.T) {
	f := seedErasure(t)
	eraseOK(t, f.repo, victim)
	for _, thread := range []string{f.t1.ID, f.t3.ID} {
		for _, m := range messagesOf(t, f.repo, thread) {
			if m.SenderID != victim {
				continue
			}
			if m.Payload != "" {
				t.Errorf("victim message %s kept payload %q", m.ID, m.Payload)
			}
			if m.Content != erasedText {
				t.Errorf("victim message %s text = %q", m.ID, m.Content)
			}
		}
	}
	if n := countWhereVictimPayload(t, f.pool); n != 0 {
		t.Fatalf("%d victim messages still carry a non-empty payload", n)
	}
	if got := messagesOf(t, f.repo, f.t4.ID)[0].Payload; got != `{"k":"v"}` {
		t.Fatalf("bystander payload changed to %q", got)
	}
}

func countWhereVictimPayload(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM chat_messages WHERE sender_id = $1 AND payload <> ''`, victim).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Scenario: The thread preview no longer shows erased text
func TestEraseReplacesThreadPreviewOnlyWhereUserSentLast(t *testing.T) {
	f := seedErasure(t)
	eraseOK(t, f.repo, victim)
	ctx := context.Background()
	get := func(id string) store.ChatThread {
		th, err := f.repo.GetThreadByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return th
	}
	if got := get(f.t1.ID).LastMessageText; got != erasedText {
		t.Errorf("t1 last_message_text = %q, want %q", got, erasedText)
	}
	if got := get(f.t3.ID).LastMessageText; got != erasedText {
		t.Errorf("t3 (user as seller) last_message_text = %q, want %q", got, erasedText)
	}
	if got := get(f.t2.ID).LastMessageText; got != "counterparty last" {
		t.Errorf("t2 last_message_text = %q, want unchanged counterparty text", got)
	}
	for _, id := range []string{f.t1.ID, f.t2.ID, f.t3.ID} {
		for _, s := range []string{"secret", "ask price", "victim seller last"} {
			if strings.Contains(get(id).LastMessageText, s) {
				t.Errorf("thread %s preview still shows %q", id, s)
			}
		}
	}
}

// Scenario: Other users' messages are untouched
func TestEraseLeavesOtherUsersUntouched(t *testing.T) {
	f := seedErasure(t)
	before := dump(t, f.pool, `SELECT m::text FROM chat_messages m WHERE thread_id = $1 ORDER BY id`, f.t4.ID)
	threadBefore := dump(t, f.pool, `SELECT th::text FROM chat_threads th WHERE id = $1`, f.t4.ID)
	eraseOK(t, f.repo, victim)
	if after := dump(t, f.pool, `SELECT m::text FROM chat_messages m WHERE thread_id = $1 ORDER BY id`, f.t4.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("bystander messages changed:\n%v\n%v", before, after)
	}
	if after := dump(t, f.pool, `SELECT th::text FROM chat_threads th WHERE id = $1`, f.t4.ID); !reflect.DeepEqual(threadBefore, after) {
		t.Fatalf("bystander thread changed:\n%v\n%v", threadBefore, after)
	}
}

// Scenario: Other users' messages are untouched (an unknown user is a no-op)
func TestEraseUnknownUserChangesNothing(t *testing.T) {
	f := seedErasure(t)
	before := snapshotAll(t, f.pool)
	if n := eraseOK(t, f.repo, "user-with-no-chat"); n != 0 {
		t.Fatalf("Erase returned %d for an unknown user, want 0", n)
	}
	if after := snapshotAll(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatal("erasing an unknown user changed rows")
	}
}

// Scenario: A replayed event changes nothing
func TestEraseReplayChangesNothing(t *testing.T) {
	f := seedErasure(t)
	eraseOK(t, f.repo, victim)
	first := snapshotAll(t, f.pool)
	if _, err := f.repo.Erase(context.Background(), victim); err != nil {
		t.Fatalf("second delivery returned an error: %v", err)
	}
	if second := snapshotAll(t, f.pool); !reflect.DeepEqual(first, second) {
		t.Fatalf("replay changed state:\n%v\n%v", first, second)
	}
}

// Scenario: A malformed event goes to the DLQ (an empty user id erases nothing)
func TestEraseEmptyUserIDRefusedAndChangesNothing(t *testing.T) {
	f := seedErasure(t)
	before := snapshotAll(t, f.pool)
	if _, err := f.repo.Erase(context.Background(), ""); err == nil {
		t.Fatal("Erase with an empty user id must fail")
	}
	if after := snapshotAll(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatal("empty user id changed rows")
	}
}

// Scenario: A store failure rolls back and is retried
func TestEraseRollsBackOnFailure(t *testing.T) {
	f := seedErasure(t)
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE FUNCTION fail_erase() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	exec(`CREATE TRIGGER fail_erase_threads BEFORE UPDATE ON chat_threads FOR EACH ROW EXECUTE FUNCTION fail_erase()`)
	before := snapshotAll(t, f.pool)
	if _, err := f.repo.Erase(ctx, victim); err == nil {
		t.Fatal("Erase must return the store failure so the record is retried")
	} else if !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("error does not carry the cause: %v", err)
	}
	if after := snapshotAll(t, f.pool); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial change survived the failure:\nbefore %v\nafter  %v", before, after)
	}
	exec(`DROP TRIGGER fail_erase_threads ON chat_threads`)
	eraseOK(t, f.repo, victim)
	var left int
	if err := f.pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM chat_messages WHERE sender_id = $1 AND content <> '%s'`, erasedText), victim).Scan(&left); err != nil || left != 0 {
		t.Fatalf("retry after recovery left %d unreplaced messages (err %v)", left, err)
	}
}
