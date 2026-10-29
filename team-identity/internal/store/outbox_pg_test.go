package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

func seedRevokeFixture(t *testing.T) (*store.PostgresSessionRepository, store.Session, func(string) int, func() bool) {
	t.Helper()
	pool := latestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u-ob1','u-ob1','x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	repo := store.NewPostgresSessionRepository(pool)
	sess, err := repo.CreateSession(ctx, store.Session{UserID: "u-ob1", Device: "d", IP: "1.1.1.1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	rowsOf := func(where string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events `+where).Scan(&n); err != nil {
			t.Fatalf("count outbox rows: %v", err)
		}
		return n
	}
	revoked := func() bool {
		var r bool
		if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&r); err != nil {
			t.Fatalf("read session: %v", err)
		}
		return r
	}
	return repo, sess, rowsOf, revoked
}

// Scenario: A revoke commits with its outbox row
func TestRevokeEnqueuesOutboxRowAtomically(t *testing.T) {
	ctx := context.Background()
	repo, sess, rowsOf, revoked := seedRevokeFixture(t)

	err := repo.RevokeSession(ctx, sess.ID, "u-ob1", func(s store.Session) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "evt-revoke-1", AggregateType: "Session", AggregateID: s.UserID, EventType: evSessionRevoked, Payload: []byte("p"), RequestID: "req-1"}, nil
	})
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if !revoked() {
		t.Fatal("session must be revoked")
	}
	if n := rowsOf(`WHERE event_id = 'evt-revoke-1' AND event_type = '` + evSessionRevoked + `' AND aggregate_id = 'u-ob1' AND status = 'pending'`); n != 1 {
		t.Fatalf("want exactly one pending SessionRevoked row for the user, found %d", n)
	}
	if n := rowsOf(`WHERE aggregate_type = 'User'`); n != 1 {
		t.Fatalf("the row must sit on the User aggregate, found %d such rows", n)
	}
	if n := rowsOf(``); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

// Scenario: An outbox failure rolls the write back
func TestRevokeRollsBackWhenOutboxFails(t *testing.T) {
	ctx := context.Background()
	repo, sess, rowsOf, revoked := seedRevokeFixture(t)

	err := repo.RevokeSession(ctx, sess.ID, "u-ob1", func(s store.Session) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "", AggregateType: "Session", AggregateID: s.UserID, EventType: evSessionRevoked, Payload: []byte("p")}, nil
	})
	if err == nil {
		t.Fatal("want an error when the outbox insert is refused")
	}
	if revoked() {
		t.Fatal("the session must stay active after an outbox failure")
	}
	if n := rowsOf(``); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}

	boom := errors.New("boom")
	err = repo.RevokeSession(ctx, sess.ID, "u-ob1", func(store.Session) (store.OutboxRow, error) { return store.OutboxRow{}, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want the builder error, got %v", err)
	}
	if revoked() {
		t.Fatal("the session must stay active after a builder failure")
	}
}

// Scenario: An outbox failure rolls the write back (status change)
func TestSetUserStatusRollsBackWhenOutboxInsertFails(t *testing.T) {
	e := pgEnv(t)
	ctx := context.Background()
	e.seedUser(t.Context(), t, "ue-oi", "ue-oi")
	e.seedSessions(t, "ue-oi", 2)

	n := 0
	revoke := func(s store.Session) (store.OutboxRow, error) {
		n++
		id := "evt-oi-1"
		if n == 2 {
			id = ""
		}
		return store.OutboxRow{EventID: id, AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(s.ID)}, nil
	}
	var changes []store.StatusChange
	_, err := e.users.SetStatus(ctx, "ue-oi", store.StatusBanned, "scam", time.Time{}, enfNow, revoke, statusBuilder(&changes))
	if err == nil {
		t.Fatal("want an error when the second outbox insert is refused")
	}
	if got := mustGet(t, e, "ue-oi"); got.Status != store.StatusActive {
		t.Errorf("user status = %q, want active after rollback", got.Status)
	}
	if c := activeSessionCount(t.Context(), t, e, "ue-oi"); c != 2 {
		t.Errorf("active sessions = %d, want 2", c)
	}
	if rows := e.outbox(t); len(rows) != 0 {
		t.Errorf("outbox rows = %d, want 0", len(rows))
	}
}
