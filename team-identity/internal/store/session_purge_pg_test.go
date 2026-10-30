package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

// Postgres tests for PurgeSessions / PurgeLoginHistory (change identity-session-retention, task 2).
// They run against a Postgres container from testkit.

const (
	purgeRetention = 720 * time.Hour
	purgeAbsMax    = 90 * 24 * time.Hour
	day            = 24 * time.Hour
)

func purgePG(t *testing.T) (*pgxpool.Pool, *store.PostgresSessionRepository) {
	t.Helper()
	ctx := context.Background()
	pool := latestPool(t)
	for _, q := range []string{
		`DELETE FROM identity_outbox_events`, `DELETE FROM sessions`, `DELETE FROM login_history`,
		`DELETE FROM users WHERE id LIKE 'u-sp%'`,
		`INSERT INTO users (id, username, password_hash) VALUES ('u-sp1','u-sp1','x')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return pool, store.NewPostgresSessionRepository(pool)
}

// sessRow is the raw column set to insert; nil pointers insert NULL.
type sessRow struct {
	id         string
	revoked    bool
	revokedAt  *time.Time
	lastSeen   time.Time
	createdAt  time.Time
	refreshExp *time.Time
}

func tp(t time.Time) *time.Time { return &t }

func insertSess(t *testing.T, pool *pgxpool.Pool, r sessRow) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO sessions
		(id, user_id, created_at, last_seen, revoked, revoked_at, refresh_expires_at)
		VALUES ($1,'u-sp1',$2,$3,$4,$5,$6)`, r.id, r.createdAt, r.lastSeen, r.revoked, r.revokedAt, r.refreshExp)
	if err != nil {
		t.Fatalf("insert session %s: %v", r.id, err)
	}
}

func sessCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions WHERE id=$1`, id).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func wantGone(t *testing.T, pool *pgxpool.Pool, id string) { t.Helper(); wantCount(t, pool, id, 0) }
func wantKept(t *testing.T, pool *pgxpool.Pool, id string) { t.Helper(); wantCount(t, pool, id, 1) }
func wantCount(t *testing.T, pool *pgxpool.Pool, id string, want int) {
	t.Helper()
	if got := sessCount(t, pool, id); got != want {
		t.Fatalf("session %s: rows = %d, want %d", id, got, want)
	}
}

func purgeNow() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func TestPurgeSessionsDeletesOldRevoked(t *testing.T) { // Session retention: revoked session older than retention is deleted
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "old-rev", revoked: true, revokedAt: tp(now.Add(-31 * day)), lastSeen: now, createdAt: now.Add(-40 * day)})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	wantGone(t, pool, "old-rev")
}

func TestPurgeSessionsDeletesExpiredRefresh(t *testing.T) { // Session retention: expired refresh session is deleted
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "exp", lastSeen: now, createdAt: now.Add(-60 * day), refreshExp: tp(now.Add(-31 * day))})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	wantGone(t, pool, "exp")
}

func TestPurgeSessionsAbsoluteMaxAge(t *testing.T) { // Session retention: absolute max age counts as expiry
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "abs", lastSeen: now, createdAt: now.Add(-121 * day), refreshExp: tp(now.Add(10 * day))})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	wantGone(t, pool, "abs")
}

func TestPurgeSessionsLegacyRows(t *testing.T) { // Session retention: legacy rows without timestamps
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "leg-rev", revoked: true, lastSeen: now.Add(-31 * day), createdAt: now.Add(-35 * day)})
	insertSess(t, pool, sessRow{id: "leg-open", lastSeen: now, createdAt: now.Add(-121 * day)})
	insertSess(t, pool, sessRow{id: "leg-young", lastSeen: now, createdAt: now.Add(-10 * day)})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	wantGone(t, pool, "leg-rev")
	wantGone(t, pool, "leg-open")
	wantKept(t, pool, "leg-young")
}

func TestPurgeSessionsKeepsActive(t *testing.T) { // Session retention: active session is never touched
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "act", lastSeen: now, createdAt: now.Add(-80 * day), refreshExp: tp(now.Add(5 * day))})
	insertSess(t, pool, sessRow{id: "exp-yday", lastSeen: now, createdAt: now.Add(-50 * day), refreshExp: tp(now.Add(-1 * day))})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted = %d, want 0", n)
	}
	wantKept(t, pool, "act")
	wantKept(t, pool, "exp-yday")
	var created, seen time.Time
	var rev bool
	if err := pool.QueryRow(context.Background(), `SELECT created_at, last_seen, revoked FROM sessions WHERE id='act'`).Scan(&created, &seen, &rev); err != nil {
		t.Fatal(err)
	}
	if !created.Equal(now.Add(-80*day)) || !seen.Equal(now) || rev {
		t.Fatalf("active session modified: created=%v seen=%v revoked=%v", created, seen, rev)
	}
}

func TestPurgeSessionsRetentionBoundary(t *testing.T) { // Session retention: boundary at exactly the retention
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "minus", revoked: true, revokedAt: tp(now.Add(-purgeRetention + time.Second)), lastSeen: now, createdAt: now.Add(-60 * day)})
	insertSess(t, pool, sessRow{id: "exact", revoked: true, revokedAt: tp(now.Add(-purgeRetention)), lastSeen: now, createdAt: now.Add(-60 * day)})
	insertSess(t, pool, sessRow{id: "plus", revoked: true, revokedAt: tp(now.Add(-purgeRetention - time.Second)), lastSeen: now, createdAt: now.Add(-60 * day)})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	wantKept(t, pool, "minus")
	wantKept(t, pool, "exact")
	wantGone(t, pool, "plus")
}

func TestPurgeSessionsKeepsRecentRevoked(t *testing.T) { // Session retention: recently revoked session is kept
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "rec", revoked: true, revokedAt: tp(now.Add(-29 * day)), lastSeen: now, createdAt: now.Add(-40 * day)})
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted = %d, want 0", n)
	}
	var rev bool
	if err := pool.QueryRow(context.Background(), `SELECT revoked FROM sessions WHERE id='rec'`).Scan(&rev); err != nil || !rev {
		t.Fatalf("recent revoked row: revoked=%v err=%v", rev, err)
	}
}

func TestPurgeSessionsHonoursLimit(t *testing.T) { // Session retention: batches of 500 (repository batch bound)
	pool, repo := purgePG(t)
	now := purgeNow()
	for _, id := range []string{"b1", "b2", "b3", "b4", "b5"} {
		insertSess(t, pool, sessRow{id: id, revoked: true, revokedAt: tp(now.Add(-40 * day)), lastSeen: now, createdAt: now.Add(-50 * day)})
	}
	n, err := repo.PurgeSessions(context.Background(), now, purgeRetention, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("first batch deleted = %d, want 2", n)
	}
	n, err = repo.PurgeSessions(context.Background(), now, purgeRetention, 2)
	if err != nil || n != 2 {
		t.Fatalf("second batch deleted = %d err=%v, want 2", n, err)
	}
	n, err = repo.PurgeSessions(context.Background(), now, purgeRetention, 2)
	if err != nil || n != 1 {
		t.Fatalf("third batch deleted = %d err=%v, want 1", n, err)
	}
}

func TestPurgeSessionsSkipsLocked(t *testing.T) { // Session retention: locked row is skipped
	pool, repo := purgePG(t)
	now := purgeNow()
	insertSess(t, pool, sessRow{id: "locked", revoked: true, revokedAt: tp(now.Add(-40 * day)), lastSeen: now, createdAt: now.Add(-50 * day)})
	insertSess(t, pool, sessRow{id: "free", revoked: true, revokedAt: tp(now.Add(-40 * day)), lastSeen: now, createdAt: now.Add(-50 * day)})

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM sessions WHERE id='locked' FOR UPDATE`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n, err := repo.PurgeSessions(pctx, now, purgeRetention, 500)
	if err != nil {
		t.Fatalf("purge must not wait on the locked row: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1 (only the unlocked row)", n)
	}
	wantGone(t, pool, "free")
	wantKept(t, pool, "locked")
}

func insertLogin(t *testing.T, pool *pgxpool.Pool, id string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO login_history (id, user_id, ip, user_agent, success, created_at)
		VALUES ($1,'u-sp1','1.2.3.4','ua',true,$2)`, id, at); err != nil {
		t.Fatalf("insert login %s: %v", id, err)
	}
}

func TestPurgeLoginHistory(t *testing.T) { // Session retention: old login history is deleted
	pool, repo := purgePG(t)
	now := purgeNow()
	insertLogin(t, pool, "lh-old", now.Add(-181*day))
	insertLogin(t, pool, "lh-new", now.Add(-179*day))
	n, err := repo.PurgeLoginHistory(context.Background(), now.Add(-180*day), 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	got, total, err := repo.ListLoginHistory(context.Background(), "u-sp1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != "lh-new" {
		t.Fatalf("history after purge = %+v total=%d, want only lh-new", got, total)
	}
}

func TestPurgeLoginHistoryBoundary(t *testing.T) { // Session retention: login history boundary
	pool, repo := purgePG(t)
	now := purgeNow()
	insertLogin(t, pool, "lh-exact", now.Add(-180*day))
	n, err := repo.PurgeLoginHistory(context.Background(), now.Add(-180*day), 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted = %d, want 0 (exactly 180 days is kept)", n)
	}
	if _, total, _ := repo.ListLoginHistory(context.Background(), "u-sp1", 10, 0); total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

func TestPurgeLoginHistoryEmptyAndLimit(t *testing.T) { // Session retention: login history without rows
	pool, repo := purgePG(t)
	now := purgeNow()
	n, err := repo.PurgeLoginHistory(context.Background(), now, 500)
	if err != nil || n != 0 {
		t.Fatalf("empty purge = %d err=%v, want 0 nil", n, err)
	}
	for _, id := range []string{"a", "b", "c"} {
		insertLogin(t, pool, "lh-"+id, now.Add(-200*day))
	}
	n, err = repo.PurgeLoginHistory(context.Background(), now.Add(-180*day), 2)
	if err != nil || n != 2 {
		t.Fatalf("limited purge = %d err=%v, want 2", n, err)
	}
}
