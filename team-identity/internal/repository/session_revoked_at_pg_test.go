package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres tests for migration 0008_session_revoked_at and the revoked_at writes in
// session.go. They run only when TEST_DATABASE_URL points at a disposable Postgres.

func revokedAtPG(t *testing.T) (*pgxpool.Pool, *PostgresSessionRepository) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres revoked_at test")
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
	for _, f := range []string{"0001_users.up.sql", "0004_sessions.up.sql", "0005_identity_outbox_events.up.sql"} {
		if _, err := pool.Exec(ctx, migrationSQL(t, f)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	if refreshColumnCount(t, pool) != 4 {
		if _, err := pool.Exec(ctx, migrationSQL(t, "0006_session_refresh.up.sql")); err != nil {
			t.Fatalf("apply 0006 up: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, migrationSQL(t, "0007_user_status.up.sql")); err != nil {
		t.Fatalf("apply 0007 up: %v", err)
	}
	if revokedAtColumnCount(t, pool) != 1 {
		if _, err := pool.Exec(ctx, migrationSQL(t, "0008_session_revoked_at.up.sql")); err != nil {
			t.Fatalf("apply 0008 up: %v", err)
		}
	}
	for _, q := range []string{`DELETE FROM identity_outbox_events`, `DELETE FROM sessions`, `DELETE FROM users WHERE id LIKE 'u-ra%'`,
		`INSERT INTO users (id, username, password_hash) VALUES ('u-ra1','u-ra1','x'), ('u-ra2','u-ra2','x')`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return pool, NewPostgresSessionRepository(pool)
}

func revokedAtColumnCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
		WHERE table_name='sessions' AND column_name='revoked_at'`).Scan(&n); err != nil {
		t.Fatalf("columns: %v", err)
	}
	return n
}

// revokedAt returns the row's revoked_at (nil when NULL) and revoked flag.
func revokedAt(t *testing.T, pool *pgxpool.Pool, id string) (*time.Time, bool) {
	t.Helper()
	var ts *time.Time
	var revoked bool
	if err := pool.QueryRow(context.Background(), `SELECT revoked_at, revoked FROM sessions WHERE id=$1`, id).Scan(&ts, &revoked); err != nil {
		t.Fatalf("read revoked_at: %v", err)
	}
	return ts, revoked
}

func wantRecent(t *testing.T, label string, ts *time.Time, revoked bool) {
	t.Helper()
	if !revoked {
		t.Fatalf("%s: session not revoked", label)
	}
	if ts == nil {
		t.Fatalf("%s: revoked_at is NULL", label)
	}
	if d := time.Since(*ts); d < -5*time.Second || d > 5*time.Second {
		t.Fatalf("%s: revoked_at %v not within 5s of now (delta %v)", label, *ts, d)
	}
}

func liveSession(t *testing.T, repo *PostgresSessionRepository, user, hash string, created time.Time) Session {
	t.Helper()
	s, err := repo.CreateSession(context.Background(), Session{
		UserID: user, RefreshHash: hash, CreatedAt: created, RefreshExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func TestRevokePathsSetRevokedAt_Postgres(t *testing.T) {
	// Scenario: Session retention: revoke records revoked_at
	pool, repo := revokedAtPG(t)
	ctx := context.Background()

	t.Run("user revoke", func(t *testing.T) {
		s := liveSession(t, repo, "u-ra1", "ra-user", time.Now().Add(-time.Hour))
		if ts, rv := revokedAt(t, pool, s.ID); ts != nil || rv {
			t.Fatalf("precondition: live session must have NULL revoked_at (ts=%v rv=%v)", ts, rv)
		}
		if err := repo.RevokeSession(ctx, s.ID, "u-ra1", nil); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		ts, rv := revokedAt(t, pool, s.ID)
		wantRecent(t, "user revoke", ts, rv)
	})

	t.Run("reuse detection", func(t *testing.T) {
		s := liveSession(t, repo, "u-ra1", "ra-reuse-1", time.Now().Add(-time.Hour))
		if res, err := repo.RotateRefresh(ctx, "ra-reuse-1", "ra-reuse-2", time.Now().Add(-time.Minute), refreshGrace, refreshMaxAge, okEv("ra-ev-rot")); err != nil || res.Outcome != RotateRotated {
			t.Fatalf("rotate: %+v %v", res, err)
		}
		res, err := repo.RotateRefresh(ctx, "ra-reuse-1", "ra-reuse-3", time.Now(), refreshGrace, refreshMaxAge, okEv("ra-ev-reuse"))
		if err != nil || res.Outcome != RotateReuse {
			t.Fatalf("want RotateReuse, got %+v %v", res, err)
		}
		ts, rv := revokedAt(t, pool, s.ID)
		wantRecent(t, "reuse", ts, rv)
	})

	t.Run("absolute lifetime", func(t *testing.T) {
		s := liveSession(t, repo, "u-ra2", "ra-life-1", time.Now().Add(-91*24*time.Hour))
		res, err := repo.RotateRefresh(ctx, "ra-life-1", "ra-life-2", time.Now(), refreshGrace, refreshMaxAge, okEv("ra-ev-life"))
		if err != nil || res.Outcome != RotateLifetimeExceeded {
			t.Fatalf("want RotateLifetimeExceeded, got %+v %v", res, err)
		}
		ts, rv := revokedAt(t, pool, s.ID)
		wantRecent(t, "lifetime", ts, rv)
	})

	t.Run("untouched sessions keep NULL", func(t *testing.T) {
		keep := liveSession(t, repo, "u-ra2", "ra-keep", time.Now().Add(-time.Hour))
		if res, err := repo.RotateRefresh(ctx, "ra-keep", "ra-keep2", time.Now(), refreshGrace, refreshMaxAge, okEv("ra-ev-keep")); err != nil || res.Outcome != RotateRotated {
			t.Fatalf("rotate: %+v %v", res, err)
		}
		if ts, rv := revokedAt(t, pool, keep.ID); ts != nil || rv {
			t.Fatalf("a rotated live session must not get revoked_at (ts=%v rv=%v)", ts, rv)
		}
	})
}

func TestRevokeSessionRolledBackLeavesNoRevokedAt_Postgres(t *testing.T) {
	// Scenario: Session retention: revoke records revoked_at (atomic with outbox)
	pool, repo := revokedAtPG(t)
	s := liveSession(t, repo, "u-ra1", "ra-rb", time.Now().Add(-time.Hour))
	err := repo.RevokeSession(context.Background(), s.ID, "u-ra1", func(Session) (OutboxRow, error) { return OutboxRow{}, errRefreshHashConflict })
	if err == nil {
		t.Fatal("want builder error")
	}
	if ts, rv := revokedAt(t, pool, s.ID); ts != nil || rv {
		t.Fatalf("rolled-back revoke must leave revoked_at NULL (ts=%v rv=%v)", ts, rv)
	}
}

func TestMigration0008Schema_Postgres(t *testing.T) {
	// Scenario: Session retention: legacy rows without timestamps
	pool, repo := revokedAtPG(t)
	ctx := context.Background()

	var nullable string
	if err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='sessions' AND column_name='revoked_at'`).Scan(&nullable); err != nil || nullable != "YES" {
		t.Fatalf("revoked_at must exist and be nullable (%q, %v)", nullable, err)
	}
	var dataType string
	if err := pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_name='sessions' AND column_name='revoked_at'`).Scan(&dataType); err != nil || dataType != "timestamp with time zone" {
		t.Fatalf("revoked_at must be timestamptz (%q, %v)", dataType, err)
	}
	var idx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='sessions' AND indexdef LIKE '%revoked%' AND indexdef LIKE '% WHERE %'`).Scan(&idx); err != nil || idx < 1 {
		t.Fatalf("want a partial index for the purge (n=%d, %v)", idx, err)
	}

	// A revoked legacy row written without revoked_at keeps NULL and stays readable.
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (id, user_id, revoked) VALUES ('ra-legacy','u-ra1', true)`); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	if ts, rv := revokedAt(t, pool, "ra-legacy"); ts != nil || !rv {
		t.Fatalf("legacy revoked row: want NULL revoked_at and revoked=true (ts=%v rv=%v)", ts, rv)
	}
	if _, err := repo.ListSessions(ctx, "u-ra1"); err != nil {
		t.Fatalf("list with legacy row: %v", err)
	}
	// Revoking an already-revoked legacy row must not error.
	if err := repo.RevokeSession(ctx, "ra-legacy", "u-ra1", nil); err != nil {
		t.Fatalf("revoke legacy: %v", err)
	}
}

func TestMigration0008Down_Postgres(t *testing.T) {
	// Scenario: Session retention: legacy rows without timestamps (reversible migration)
	pool, _ := revokedAtPG(t)
	ctx := context.Background()
	if revokedAtColumnCount(t, pool) != 1 {
		t.Fatalf("precondition: revoked_at column missing")
	}
	if _, err := pool.Exec(ctx, migrationSQL(t, "0008_session_revoked_at.down.sql")); err != nil {
		t.Fatalf("down: %v", err)
	}
	t.Cleanup(func() {
		if revokedAtColumnCount(t, pool) == 0 {
			_, _ = pool.Exec(ctx, migrationSQL(t, "0008_session_revoked_at.up.sql"))
		}
	})
	if n := revokedAtColumnCount(t, pool); n != 0 {
		t.Fatalf("revoked_at left after down: %d", n)
	}
	var idx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='sessions' AND indexdef LIKE '%revoked_at%'`).Scan(&idx); err != nil || idx != 0 {
		t.Fatalf("indexes left after down: %d %v", idx, err)
	}
	if _, err := pool.Exec(ctx, migrationSQL(t, "0008_session_revoked_at.up.sql")); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if revokedAtColumnCount(t, pool) != 1 {
		t.Fatal("column missing after re-up")
	}
}

func TestSetStatusBulkRevokeSetsRevokedAt_Postgres(t *testing.T) {
	// Scenario: Session retention: revoke records revoked_at (admin suspend/ban bulk revoke path)
	pool, repo := revokedAtPG(t)
	ctx := context.Background()
	s := liveSession(t, repo, "u-ra2", "ra-setstatus", time.Now().Add(-time.Hour))
	revoke := func(s Session) (OutboxRow, error) {
		return OutboxRow{EventID: "ra-ss-rev-" + s.ID, AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(s.ID)}, nil
	}
	changed := func(c StatusChange) (OutboxRow, error) {
		return OutboxRow{EventID: "ra-ss-status", AggregateType: "User", AggregateID: c.UserID, EventType: evStatus, Payload: []byte("x")}, nil
	}
	if _, err := NewPostgresUserRepository(pool).SetStatus(ctx, "u-ra2", StatusBanned, "scam", time.Time{}, time.Now(), revoke, changed); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	ts, rv := revokedAt(t, pool, s.ID)
	wantRecent(t, "setstatus bulk revoke", ts, rv)
}
