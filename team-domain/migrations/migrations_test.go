package migrations_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/migrations"
)

func newDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return dsn, pool
}

func migrationVersion(name string) int {
	n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
	if err != nil {
		return -1
	}
	return n
}

func filesUpTo(t *testing.T, max int) fs.FS {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	m := fstest.MapFS{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") || migrationVersion(e.Name()) > max {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		m[e.Name()] = &fstest.MapFile{Data: b}
	}
	return m
}

type schemaVersion struct {
	version int
	dirty   bool
}

func readVersion(t *testing.T, pool *pgxpool.Pool) schemaVersion {
	t.Helper()
	var v schemaVersion
	if err := pool.QueryRow(t.Context(), `SELECT version, dirty FROM schema_migrations`).Scan(&v.version, &v.dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v
}

func TestMigrateUpBuildsV1Outbox(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, os.DirFS(".")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	ctx := t.Context()

	if v := readVersion(t, pool); v.version != 16 || v.dirty {
		t.Errorf("schema_migrations = %+v, want version 16 and clean", v)
	}

	type column struct {
		nullable string
		def      string
	}
	cols := map[string]column{}
	rows, err := pool.Query(ctx, `SELECT column_name, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'outbox_events'`)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	for rows.Next() {
		var name string
		var c column
		if err := rows.Scan(&name, &c.nullable, &c.def); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols[name] = c
	}
	rows.Close()
	for _, name := range []string{"topic", "traceparent", "seq"} {
		if _, ok := cols[name]; !ok {
			t.Errorf("outbox_events has no column %q", name)
		}
	}
	if c := cols["seq"]; c.nullable != "NO" || !strings.HasPrefix(c.def, "nextval(") {
		t.Errorf("seq = %+v, want NOT NULL with a nextval default", c)
	}

	idx := map[string]bool{}
	irows, err := pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'outbox_events'`)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	for irows.Next() {
		var n string
		if err := irows.Scan(&n); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		idx[n] = true
	}
	irows.Close()
	for _, want := range []string{"outbox_events_agg_idx", "outbox_events_pending_idx"} {
		if !idx[want] {
			t.Errorf("index %s missing (have %v)", want, idx)
		}
	}
	if idx["outbox_events_claim_idx"] {
		t.Errorf("legacy index outbox_events_claim_idx must be dropped")
	}
}

func insertOld(t *testing.T, pool *pgxpool.Pool, id, aggregate, status string, attempts int, errText *string, createdAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, error, available_at, created_at)
		VALUES ($1, 'Listing', $2, 'platform.listing.v1.ListingChanged', '\x01', '', $3, $4, $5, $6, $6)`,
		id, aggregate, status, attempts, errText, createdAt); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func TestOutboxUpgradeNumbersRowsByCreatedAt(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, filesUpTo(t, 12)); err != nil {
		t.Fatalf("migrate to 12: %v", err)
	}
	if v := readVersion(t, pool); v.version != 12 {
		t.Fatalf("version before upgrade = %d, want 12", v.version)
	}
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id  string
		age time.Duration
	}{
		{"e-d", 3 * time.Minute},
		{"e-a", 1 * time.Minute},
		{"e-e", 4 * time.Minute},
		{"e-c", 2 * time.Minute},
		{"e-b", 2 * time.Minute},
	} {
		insertOld(t, pool, r.id, "L1", "pending", 0, nil, base.Add(r.age))
	}

	if err := migrate.Up(t.Context(), dsn, os.DirFS(".")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	rows, err := pool.Query(t.Context(), `SELECT event_id FROM outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, id)
	}
	want := []string{"e-a", "e-b", "e-c", "e-d", "e-e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rows in seq order = %v, want (created_at, event_id) order %v", got, want)
	}
	var distinct int
	if err := pool.QueryRow(t.Context(), `SELECT count(DISTINCT seq) FROM outbox_events`).Scan(&distinct); err != nil || distinct != 5 {
		t.Fatalf("distinct seq = %d err=%v, want 5", distinct, err)
	}
}

func TestOutboxUpgradeRequeuesFailedRows(t *testing.T) {
	dsn, pool := newDatabase(t)
	if err := migrate.Up(t.Context(), dsn, filesUpTo(t, 12)); err != nil {
		t.Fatalf("migrate to 12: %v", err)
	}
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	reason := "max attempts"
	insertOld(t, pool, "f-1", "LA", "failed", 5, &reason, base)
	insertOld(t, pool, "p-1", "LA", "pending", 0, nil, base.Add(time.Minute))
	insertOld(t, pool, "p-2", "LB", "pending", 0, nil, base.Add(2*time.Minute))
	insertOld(t, pool, "d-1", "LC", "published", 1, nil, base.Add(3*time.Minute))

	if err := migrate.Up(t.Context(), dsn, os.DirFS(".")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	type state struct {
		status   string
		attempts int
		err      *string
	}
	got := map[string]state{}
	rows, err := pool.Query(t.Context(), `SELECT event_id, status, attempts, error FROM outbox_events`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s state
		if err := rows.Scan(&id, &s.status, &s.attempts, &s.err); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[id] = s
	}
	for _, id := range []string{"f-1", "p-1", "p-2"} {
		if s := got[id]; s.status != "pending" || s.attempts != 0 || s.err != nil {
			t.Errorf("%s = %+v, want pending, attempts 0, error cleared", id, s)
		}
	}
	if s := got["d-1"]; s.status != "published" {
		t.Errorf("published row changed to %+v", s)
	}
}

func TestMigrateUpIsIdempotent(t *testing.T) {
	dsn, pool := newDatabase(t)
	fsys := os.DirFS(".")
	if err := migrate.Up(t.Context(), dsn, fsys); err != nil {
		t.Fatalf("first migrate up: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status)
		VALUES ('keep-1', 'Listing', 'L1', 'T', '\x01', '', 'pending')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var seqBefore int64
	if err := pool.QueryRow(t.Context(), `SELECT seq FROM outbox_events WHERE event_id = 'keep-1'`).Scan(&seqBefore); err != nil {
		t.Fatalf("seq: %v", err)
	}
	before := readVersion(t, pool)

	if err := migrate.Up(t.Context(), dsn, fsys); err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	if after := readVersion(t, pool); after != before || after.version != 16 {
		t.Errorf("schema_migrations %+v -> %+v, want unchanged at 16", before, after)
	}
	var n int
	var seqAfter int64
	if err := pool.QueryRow(t.Context(), `SELECT count(*), max(seq) FROM outbox_events`).Scan(&n, &seqAfter); err != nil {
		t.Fatalf("recount: %v", err)
	}
	if n != 1 || seqAfter != seqBefore {
		t.Errorf("outbox contents changed: rows=%d seq %d -> %d", n, seqBefore, seqAfter)
	}
}

func TestEmbeddedFSHoldsEverySQLFile(t *testing.T) {
	onDisk, err := filepath.Glob("*.sql")
	if err != nil || len(onDisk) == 0 {
		t.Fatalf("glob: %v (n=%d)", err, len(onDisk))
	}
	sort.Strings(onDisk)
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded FS: %v", err)
	}
	var embedded []string
	for _, e := range entries {
		embedded = append(embedded, e.Name())
	}
	sort.Strings(embedded)
	if strings.Join(embedded, ",") != strings.Join(onDisk, ",") {
		t.Fatalf("embedded files %v, want every *.sql on disk %v", embedded, onDisk)
	}
	for _, name := range []string{
		"0013_outbox_v1_columns", "0014_outbox_v1_backfill", "0015_outbox_v1_switch", "0016_outbox_v1_finish",
	} {
		for _, dir := range []string{".up.sql", ".down.sql"} {
			if _, err := fs.Stat(migrations.FS, name+dir); err != nil {
				t.Errorf("embedded FS lacks %s%s: %v", name, dir, err)
			}
		}
	}
}
