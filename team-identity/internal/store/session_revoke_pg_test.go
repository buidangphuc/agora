package store

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
)

// The revoke and its outbox row are one transaction: a failing event builder rolls
// the revoke back; a successful one commits both. Runs against a disposable
// Postgres (testkit).
func TestRevokeSessionIsAtomicWithOutbox_Postgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	for _, q := range []string{`DELETE FROM identity_outbox_events`, `DELETE FROM sessions`, `DELETE FROM users WHERE id = 'u-revoke'`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u-revoke', 'u-revoke', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewPostgresSessionRepository(pool)
	sess, err := repo.CreateSession(ctx, Session{UserID: "u-revoke", Device: "d", IP: "1.1.1.1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A failing event builder must roll the revoke back.
	boom := errors.New("boom")
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (outbox.Row, error) { return outbox.Row{}, boom }); !errors.Is(err, boom) {
		t.Fatalf("want builder error, got %v", err)
	}
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("revoke must be rolled back (revoked=%v err=%v)", revoked, err)
	}

	// An outbox INSERT that fails is also a rollback. A NULL payload violates NOT
	// NULL; a duplicate event_id no longer fails because the SDK enqueue ignores it.
	row := outbox.Row{EventID: "evt-1", AggregateType: "Session", AggregateID: "u-revoke", EventType: "T", Payload: []byte{0}}
	bad := row
	bad.Payload = nil
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (outbox.Row, error) { return bad, nil }); err == nil {
		t.Fatal("want an error from the failing outbox insert")
	}
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("revoke must be rolled back on outbox failure (revoked=%v err=%v)", revoked, err)
	}

	// Success commits both.
	row.EventID = "evt-2"
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (outbox.Row, error) { return row, nil }); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE event_id = 'evt-2'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("want the outbox row committed (n=%d err=%v)", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("session must be revoked (revoked=%v err=%v)", revoked, err)
	}
	if err := repo.RevokeSession(ctx, "nope", "u-revoke", nil); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}
