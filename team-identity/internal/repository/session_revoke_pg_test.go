package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The revoke and its outbox row are one transaction: a failing event builder rolls
// the revoke back; a successful one commits both. Runs only against a disposable
// Postgres (TEST_DATABASE_URL).
func TestRevokeSessionIsAtomicWithOutbox_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres revoke atomicity test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	for _, f := range []string{"0001_users.up.sql", "0004_sessions.up.sql", "0005_identity_outbox_events.up.sql"} {
		ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
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
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (OutboxRow, error) { return OutboxRow{}, boom }); !errors.Is(err, boom) {
		t.Fatalf("want builder error, got %v", err)
	}
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("revoke must be rolled back (revoked=%v err=%v)", revoked, err)
	}

	// A duplicate event_id makes the outbox INSERT fail: also a rollback.
	row := OutboxRow{EventID: "evt-1", AggregateType: "Session", AggregateID: "u-revoke", EventType: "T", Payload: []byte{0}}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload) VALUES ('evt-1','Session','u-revoke','T','\x00')`); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (OutboxRow, error) { return row, nil }); err == nil {
		t.Fatal("want an error from the duplicate outbox insert")
	}
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id = $1`, sess.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("revoke must be rolled back on outbox failure (revoked=%v err=%v)", revoked, err)
	}

	// Success commits both.
	row.EventID = "evt-2"
	if err := repo.RevokeSession(ctx, sess.ID, "u-revoke", func(Session) (OutboxRow, error) { return row, nil }); err != nil {
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
