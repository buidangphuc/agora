package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

// Postgres tests for migration 0006_session_refresh and RotateRefresh. They run
// against a Postgres container from testkit. Shares the
// expectations documented in session_refresh_test.go.

func refreshColumnCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
		WHERE table_name='sessions' AND column_name IN ('refresh_hash','prev_refresh_hash','refresh_expires_at','rotated_at')`).Scan(&n)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	return n
}

func refreshIndexCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_indexes
		WHERE tablename='sessions' AND indexdef LIKE '%refresh_hash%'`).Scan(&n)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	return n
}

func refreshPG(t *testing.T) (*pgxpool.Pool, *store.PostgresSessionRepository) {
	t.Helper()
	ctx := context.Background()
	pool := latestPool(t)
	for _, q := range []string{
		`DELETE FROM identity_outbox_events`, `DELETE FROM sessions`, `DELETE FROM users WHERE id LIKE 'u-rf%'`,
		`INSERT INTO users (id, username, password_hash) VALUES ('u-rf1','u-rf1','x'), ('u-rf2','u-rf2','x')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return pool, store.NewPostgresSessionRepository(pool)
}

func newPGSession(t *testing.T, repo *store.PostgresSessionRepository, user, hash string) store.Session {
	t.Helper()
	s, err := repo.CreateSession(context.Background(), store.Session{
		UserID: user, RefreshHash: hash, RefreshExpiresAt: refreshT0.Add(30 * 24 * time.Hour), CreatedAt: refreshT0.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func TestRefreshHashOnlyStored(t *testing.T) {
	pool, repo := refreshPG(t)
	const rawToken = "RAW-TOKEN-SHOULD-NEVER-BE-STORED-0123456789"
	const hexHash = "3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b"
	s := newPGSession(t, repo, "u-rf1", hexHash)
	var rowJSON, stored string
	if err := pool.QueryRow(context.Background(), `SELECT to_jsonb(sessions)::text, refresh_hash FROM sessions WHERE id=$1`, s.ID).Scan(&rowJSON, &stored); err != nil {
		t.Fatal(err)
	}
	if stored != hexHash || strings.Contains(rowJSON, rawToken) {
		t.Fatalf("stored=%q row=%s", stored, rowJSON)
	}
}

func TestRefreshHashUnique_DuplicateHashIsRefused_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	newPGSession(t, repo, "u-rf1", "dup")
	_, err := repo.CreateSession(context.Background(), store.Session{UserID: "u-rf2", RefreshHash: "dup"})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("want unique violation 23505, got %v", err)
	}
}

func TestLegacySessionUnaffectedByRefreshColumns_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	// Row as written before the migration: refresh columns untouched (NULL).
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (id, user_id) VALUES ('legacy-1','u-rf1')`); err != nil {
		t.Fatal(err)
	}
	// Sessions without a hash must not collide on the unique index (stored as NULL).
	for i := 0; i < 2; i++ {
		if _, err := repo.CreateSession(ctx, store.Session{UserID: "u-rf1"}); err != nil {
			t.Fatalf("hashless create #%d: %v", i, err)
		}
	}
	items, err := repo.ListSessions(ctx, "u-rf1")
	if err != nil || len(items) != 3 {
		t.Fatalf("list: n=%d err=%v", len(items), err)
	}
	for _, p := range []string{"", "anything"} {
		res, err := repo.RotateRefresh(ctx, p, "n", refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("e"))
		if err != nil || res.Outcome != store.RotateStale {
			t.Fatalf("presented %q: %v %v", p, res.Outcome, err)
		}
	}
	if err := repo.RevokeSession(ctx, "legacy-1", "u-rf1", nil); err != nil {
		t.Fatalf("revoke legacy: %v", err)
	}
}

func TestMigration0006Down_Postgres(t *testing.T) {
	pool, _ := refreshPG(t)
	ctx := context.Background()
	if refreshColumnCount(t, pool) != 4 || refreshIndexCount(t, pool) < 1 {
		t.Fatalf("precondition: want 4 columns and the refresh_hash index")
	}
	if _, err := pool.Exec(ctx, migrationSQL(t, "0006_session_refresh.down.sql")); err != nil {
		t.Fatalf("down: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, migrationSQL(t, "0006_session_refresh.up.sql")) })
	if n := refreshColumnCount(t, pool); n != 0 {
		t.Fatalf("columns left after down: %d", n)
	}
	var idx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='sessions' AND (indexdef LIKE '%refresh_hash%' OR indexdef LIKE '%rotated_at%' OR indexdef LIKE '%refresh_expires_at%')`).Scan(&idx); err != nil || idx != 0 {
		t.Fatalf("indexes left after down: %d %v", idx, err)
	}
	// Up again must work (reversible).
	if _, err := pool.Exec(ctx, migrationSQL(t, "0006_session_refresh.up.sql")); err != nil {
		t.Fatalf("re-up: %v", err)
	}
}

func TestRotateRefresh_Lifecycle_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "h1")
	other := newPGSession(t, repo, "u-rf2", "o1")

	res, err := repo.RotateRefresh(ctx, "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("unused"))
	if err != nil || res.Outcome != store.RotateRotated || res.Session.ID != s.ID {
		t.Fatalf("rotate: %+v %v", res, err)
	}
	var h, p string
	var rot time.Time
	q := func() {
		if err := pool.QueryRow(ctx, `SELECT refresh_hash, COALESCE(prev_refresh_hash,''), rotated_at FROM sessions WHERE id=$1`, s.ID).Scan(&h, &p, &rot); err != nil {
			t.Fatal(err)
		}
	}
	q()
	if h != "h2" || p != "h1" || !rot.Equal(refreshT0) {
		t.Fatalf("row: %q %q %v", h, p, rot)
	}
	var oh string
	if err := pool.QueryRow(ctx, `SELECT refresh_hash FROM sessions WHERE id=$1`, other.ID).Scan(&oh); err != nil || oh != "o1" {
		t.Fatalf("other session changed: %q %v", oh, err)
	}

	// Grace boundary, inclusive.
	if res, err = repo.RotateRefresh(ctx, "h1", "hx", refreshT0.Add(10*time.Second), refreshGrace, refreshMaxAge, "", "", okEv("unused")); err != nil || res.Outcome != store.RotateReplayedInGrace {
		t.Fatalf("grace: %+v %v", res, err)
	}
	q()
	if h != "h2" || p != "h1" {
		t.Fatalf("grace changed row: %q %q", h, p)
	}

	// Stale after a second rotation.
	if res, err = repo.RotateRefresh(ctx, "h2", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge, "", "", okEv("n")); err != nil || res.Outcome != store.RotateRotated {
		t.Fatalf("rotate2: %+v %v", res, err)
	}
	if res, err = repo.RotateRefresh(ctx, "h1", "hy", refreshT0.Add(time.Minute+time.Second), refreshGrace, refreshMaxAge, "", "", okEv("unused")); err != nil || res.Outcome != store.RotateStale {
		t.Fatalf("stale: %+v %v", res, err)
	}
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked FROM sessions WHERE id=$1`, s.ID).Scan(&revoked); err != nil || revoked {
		t.Fatalf("stale must not revoke: %v %v", revoked, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no outbox rows expected: %d %v", n, err)
	}
}

func TestReuseWritesSessionRevokedOutbox(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "h1")
	if _, err := repo.RotateRefresh(ctx, "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("n")); err != nil {
		t.Fatal(err)
	}
	res, err := repo.RotateRefresh(ctx, "h1", "h3", refreshT0.Add(11*time.Second), refreshGrace, refreshMaxAge, "", "", okEv("evt-reuse-1"))
	if err != nil || res.Outcome != store.RotateReuse {
		t.Fatalf("reuse: %+v %v", res, err)
	}
	var revoked bool
	var rh, ph *string
	if err := pool.QueryRow(ctx, `SELECT revoked, refresh_hash, prev_refresh_hash FROM sessions WHERE id=$1`, s.ID).Scan(&revoked, &rh, &ph); err != nil {
		t.Fatal(err)
	}
	if !revoked || rh != nil || ph != nil {
		t.Fatalf("want revoked with NULL hashes: %v %v %v", revoked, rh, ph)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE event_id='evt-reuse-1' AND event_type='platform.identity.v1.SessionRevoked' AND aggregate_id='u-rf1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("want exactly one outbox row: %d %v", n, err)
	}
	// R2 and replayed R1 are now unknown.
	for _, p := range []string{"h2", "h1"} {
		if r, err := repo.RotateRefresh(ctx, p, "z", refreshT0.Add(12*time.Second), refreshGrace, refreshMaxAge, "", "", okEv("evt-x")); err != nil || r.Outcome != store.RotateStale {
			t.Fatalf("%s after reuse: %+v %v", p, r, err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("no further outbox rows: %d %v", n, err)
	}
}

func TestReuseOutboxFailureRollsBack(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "h1")
	if _, err := repo.RotateRefresh(ctx, "h1", "h2", refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("n")); err != nil {
		t.Fatal(err)
	}
	dup := func(store.Session) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "", AggregateType: "Session", AggregateID: "u-rf1", EventType: "T", Payload: []byte{0}}, nil
	}
	boom := errors.New("boom")
	for _, ev := range []store.EnqueueFn{dup, func(store.Session) (store.OutboxRow, error) { return store.OutboxRow{}, boom }} {
		res, err := repo.RotateRefresh(ctx, "h1", "h3", refreshT0.Add(time.Minute), refreshGrace, refreshMaxAge, "", "", ev)
		if err == nil || res.Outcome == store.RotateReuse && err == nil {
			t.Fatalf("want an error, got %+v %v", res, err)
		}
		if !errors.Is(err, store.ErrRotateFault) {
			t.Fatalf("want errors.Is ErrRotateFault, got %v", err)
		}
		var revoked bool
		var rh, ph string
		if err := pool.QueryRow(ctx, `SELECT revoked, refresh_hash, prev_refresh_hash FROM sessions WHERE id=$1`, s.ID).Scan(&revoked, &rh, &ph); err != nil {
			t.Fatal(err)
		}
		if revoked || rh != "h2" || ph != "h1" {
			t.Fatalf("must roll back: %v %q %q", revoked, rh, ph)
		}
	}
}

func TestRefreshConcurrentSameToken_Postgres(t *testing.T) {
	_, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "h1")
	const workers = 8
	var wg sync.WaitGroup
	results := make([]store.RotateResult, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.RotateRefresh(ctx, "h1", "new-"+string(rune('a'+i)), refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("evt-"+string(rune('a'+i))))
		}(i)
	}
	close(start)
	wg.Wait()
	rotated, grace := 0, 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		switch results[i].Outcome {
		case store.RotateRotated:
			rotated++
		case store.RotateReplayedInGrace:
			grace++
		default:
			t.Fatalf("worker %d outcome %v", i, results[i].Outcome)
		}
	}
	if rotated != 1 || grace != workers-1 {
		t.Fatalf("rotated=%d grace=%d", rotated, grace)
	}
	got := getSession(t, repo, "u-rf1", s.ID)
	if got.Revoked || got.PrevRefreshHash != "h1" || !strings.HasPrefix(got.RefreshHash, "new-") {
		t.Fatalf("session: %+v", got)
	}
}

func TestRevokeSessionClearsHashes_EveryPath_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	for i, ev := range []store.EnqueueFn{nil, okEv("evt-rv")} {
		s := newPGSession(t, repo, "u-rf1", "rv-"+string(rune('a'+i)))
		if _, err := repo.RotateRefresh(ctx, "rv-"+string(rune('a'+i)), "rv2-"+string(rune('a'+i)), refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("n")); err != nil {
			t.Fatal(err)
		}
		if err := repo.RevokeSession(ctx, s.ID, "u-rf1", ev); err != nil {
			t.Fatal(err)
		}
		var rh, ph *string
		if err := pool.QueryRow(ctx, `SELECT refresh_hash, prev_refresh_hash FROM sessions WHERE id=$1`, s.ID).Scan(&rh, &ph); err != nil || rh != nil || ph != nil {
			t.Fatalf("path %d: hashes %v %v err=%v", i, rh, ph, err)
		}
	}
}

// --- round 2: expiry, absolute lifetime, mandatory ev, error hygiene ---

func pgSetTimes(t *testing.T, pool *pgxpool.Pool, id string, createdAt, expiresAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE sessions SET created_at=$2, refresh_expires_at=$3 WHERE id=$1`, id, createdAt, expiresAt); err != nil {
		t.Fatal(err)
	}
}

type pgRow struct {
	Revoked          bool
	RH, PH           string
	Exp, Rot, LastSn time.Time
}

func pgRowOf(t *testing.T, pool *pgxpool.Pool, id string) pgRow {
	t.Helper()
	var r pgRow
	var rot *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT revoked, COALESCE(refresh_hash,''), COALESCE(prev_refresh_hash,''), refresh_expires_at, rotated_at, last_seen FROM sessions WHERE id=$1`, id).
		Scan(&r.Revoked, &r.RH, &r.PH, &r.Exp, &rot, &r.LastSn); err != nil {
		t.Fatal(err)
	}
	if rot != nil {
		r.Rot = *rot
	}
	return r
}

func outboxCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_outbox_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRotateRefresh_ExpiredLiveToken_NoChange_RefreshTokenExpired_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	for i, exp := range []time.Time{refreshT0, refreshT0.Add(-time.Second)} {
		h := "ex-" + string(rune('a'+i))
		s := newPGSession(t, repo, "u-rf1", h)
		pgSetTimes(t, pool, s.ID, refreshT0.Add(-time.Hour), exp)
		before := pgRowOf(t, pool, s.ID)
		p := &evProbe{}
		res, err := repo.RotateRefresh(ctx, h, "new-"+h, refreshT0, refreshGrace, refreshMaxAge, "", "", p.fn("e"))
		if err != nil || res.Outcome != store.RotateExpired {
			t.Fatalf("%s: %+v %v", h, res, err)
		}
		if after := pgRowOf(t, pool, s.ID); after != before {
			t.Fatalf("%s row changed: %+v -> %+v", h, before, after)
		}
		if p.calls != 0 || outboxCount(t, pool) != 0 {
			t.Fatalf("%s: ev calls=%d", h, p.calls)
		}
	}
}

func TestRotateRefresh_ExpiredSessionGraceReplay_NoChange_RefreshTokenExpired_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "g1")
	if res, err := repo.RotateRefresh(ctx, "g1", "g2", refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("x")); err != nil || res.Outcome != store.RotateRotated {
		t.Fatalf("rotate: %+v %v", res, err)
	}
	pgSetTimes(t, pool, s.ID, refreshT0.Add(-time.Hour), refreshT0.Add(5*time.Second))
	before := pgRowOf(t, pool, s.ID)
	p := &evProbe{}
	res, err := repo.RotateRefresh(ctx, "g1", "g3", refreshT0.Add(6*time.Second), refreshGrace, refreshMaxAge, "", "", p.fn("e"))
	if err != nil || res.Outcome != store.RotateExpired {
		t.Fatalf("grace replay on expired: %+v %v", res, err)
	}
	if after := pgRowOf(t, pool, s.ID); after != before || p.calls != 0 || outboxCount(t, pool) != 0 {
		t.Fatalf("must be unchanged: %+v calls=%d", after, p.calls)
	}
}

func TestRotateRefresh_AbsoluteLifetimeExceeded_RevokesWithOneOutboxRow_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "l1")
	pgSetTimes(t, pool, s.ID, refreshT0.Add(-refreshMaxAge-time.Millisecond), refreshT0.Add(time.Hour))
	p := &evProbe{}
	res, err := repo.RotateRefresh(ctx, "l1", "l2", refreshT0, refreshGrace, refreshMaxAge, "", "", p.fn("evt-life-pg"))
	if err != nil || res.Outcome != store.RotateLifetimeExceeded {
		t.Fatalf("outcome: %+v %v", res, err)
	}
	if got := pgRowOf(t, pool, s.ID); !got.Revoked || got.RH != "" || got.PH != "" {
		t.Fatalf("want revoked + NULL hashes: %+v", got)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE event_id='evt-life-pg' AND event_type='platform.identity.v1.SessionRevoked' AND aggregate_id='u-rf1'`).Scan(&n); err != nil || n != 1 || p.calls != 1 {
		t.Fatalf("outbox=%d calls=%d err=%v", n, p.calls, err)
	}
	if r, err := repo.RotateRefresh(ctx, "l1", "l3", refreshT0.Add(time.Second), refreshGrace, refreshMaxAge, "", "", okEv("again")); err != nil || r.Outcome != store.RotateStale {
		t.Fatalf("after revoke: %+v %v", r, err)
	}
	if outboxCount(t, pool) != 1 {
		t.Fatal("exactly one outbox row expected")
	}
}

func TestRotateRefresh_ExactlyMaxAge_StillRotates_AbsoluteLifetimeBoundary_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	s := newPGSession(t, repo, "u-rf1", "b1")
	pgSetTimes(t, pool, s.ID, refreshT0.Add(-refreshMaxAge), refreshT0.Add(time.Hour))
	p := &evProbe{}
	res, err := repo.RotateRefresh(context.Background(), "b1", "b2", refreshT0, refreshGrace, refreshMaxAge, "", "", p.fn("e"))
	if err != nil || res.Outcome != store.RotateRotated || p.calls != 0 {
		t.Fatalf("%+v %v calls=%d", res, err, p.calls)
	}
	if got := pgRowOf(t, pool, s.ID); got.Revoked || got.RH != "b2" {
		t.Fatalf("row: %+v", got)
	}
}

func TestRotateRefresh_LifetimeExceededOutboxFailureRollsBack_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	ctx := context.Background()
	s := newPGSession(t, repo, "u-rf1", "r1")
	pgSetTimes(t, pool, s.ID, refreshT0.Add(-refreshMaxAge-time.Second), refreshT0.Add(time.Hour))
	before := pgRowOf(t, pool, s.ID)
	dup := func(store.Session) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "", AggregateType: "Session", AggregateID: "u-rf1", EventType: "T", Payload: []byte{0}}, nil
	}
	boom := errors.New("boom")
	for _, ev := range []store.EnqueueFn{dup, func(store.Session) (store.OutboxRow, error) { return store.OutboxRow{}, boom }} {
		if _, err := repo.RotateRefresh(ctx, "r1", "r2", refreshT0, refreshGrace, refreshMaxAge, "", "", ev); err == nil {
			t.Fatal("want error")
		}
		if after := pgRowOf(t, pool, s.ID); after != before {
			t.Fatalf("must roll back: %+v", after)
		}
	}
	if n := outboxCount(t, pool); n != 0 {
		t.Fatalf("failed attempts must leave no outbox row, got %d", n)
	}
}

func TestRotateRefresh_NilEvIsRejected_NoChange_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	s := newPGSession(t, repo, "u-rf1", "n1")
	before := pgRowOf(t, pool, s.ID)
	if _, err := repo.RotateRefresh(context.Background(), "n1", "n2", refreshT0, refreshGrace, refreshMaxAge, "", "", nil); err == nil {
		t.Fatal("want error for nil ev")
	}
	if after := pgRowOf(t, pool, s.ID); after != before {
		t.Fatalf("row changed: %+v", after)
	}
}

func TestRotateRefresh_UniqueViolationErrorDoesNotLeakHash_Postgres(t *testing.T) {
	pool, repo := refreshPG(t)
	const secretHash = "LEAKCHECK-deadbeefcafef00d-0123456789abcdef"
	s1 := newPGSession(t, repo, "u-rf1", "k1")
	newPGSession(t, repo, "u-rf2", secretHash)
	before := pgRowOf(t, pool, s1.ID)
	_, err := repo.RotateRefresh(context.Background(), "k1", secretHash, refreshT0, refreshGrace, refreshMaxAge, "", "", okEv("e"))
	if err == nil {
		t.Fatal("want unique-violation error")
	}
	if strings.Contains(err.Error(), secretHash) {
		t.Fatalf("error leaks the hash: %v", err)
	}
	if !errors.Is(err, store.ErrRotateFault) {
		t.Fatalf("hash conflict must wrap ErrRotateFault, got %v", err)
	}
	if after := pgRowOf(t, pool, s1.ID); after != before {
		t.Fatalf("row changed: %+v", after)
	}
}
