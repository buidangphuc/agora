package store_test

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	evSessionRevoked = "platform.identity.v1.SessionRevoked"
	evUserStatus     = "platform.identity.v1.UserStatusChanged"
)

var migrationFile = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.(up|down)\.sql$`)

func migrationFiles(t *testing.T) map[string]map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(store.Migrations(), ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	out := map[string]map[string]string{}
	for _, e := range entries {
		m := migrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("migration file %s does not match NNNN_name.(up|down).sql", e.Name())
			continue
		}
		if out[m[1]] == nil {
			out[m[1]] = map[string]string{}
		}
		out[m[1]][m[3]] = e.Name()
	}
	return out
}

func statementsOf(sql string) []string {
	var kept []string
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		kept = append(kept, line)
	}
	var out []string
	for _, st := range strings.Split(strings.Join(kept, "\n"), ";") {
		if n := strings.Join(strings.Fields(st), " "); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func normalised(sql string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.Join(strings.Fields(sql), " ")), ";"))
}

// Scenario: The outbox migrations equal the SDK upgrade steps
func TestOutboxMigrationsEqualUpgradeSQL(t *testing.T) {
	up, err := outbox.UpgradeSQL(outboxTable)
	if err != nil {
		t.Fatalf("upgrade sql: %v", err)
	}
	var want []string
	for _, st := range up.Prepare {
		want = append(want, normalised(st))
	}
	want = append(want, normalised(up.Backfill))
	for _, st := range up.Switch {
		want = append(want, normalised(st))
	}
	for _, st := range up.Finish {
		want = append(want, normalised(st))
	}

	files := migrationFiles(t)
	var got []string
	for v := 9; v <= 18; v++ {
		ver := pad4(v)
		name := files[ver]["up"]
		if name == "" || files[ver]["down"] == "" {
			t.Errorf("migration %s needs an up and a down file, has %v", ver, files[ver])
			continue
		}
		b, err := fs.ReadFile(store.Migrations(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		stmts := statementsOf(string(b))
		if len(stmts) == 0 {
			t.Errorf("%s holds no statement", name)
		}
		if strings.Contains(strings.ToUpper(string(b)), "CONCURRENTLY") && len(stmts) != 1 {
			t.Errorf("%s: a concurrent index statement must be alone in its file, found %d statements", name, len(stmts))
		}
		got = append(got, stmts...)
	}
	if t.Failed() {
		return
	}
	if len(got) != len(want) {
		t.Fatalf("migrations 0009-0018 hold %d statements, the SDK upgrade has %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d differs from the SDK upgrade:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

func TestOutboxMigrationFileNames(t *testing.T) {
	files := migrationFiles(t)
	var versions []string
	for v := range files {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	if len(versions) != 19 || versions[0] != "0001" || versions[18] != "0019" {
		t.Fatalf("want contiguous migrations 0001..0019, found %v", versions)
	}
	for i, v := range versions {
		if v != pad4(i+1) {
			t.Fatalf("migration versions are not contiguous: %v", versions)
		}
		if files[v]["up"] == "" || files[v]["down"] == "" {
			t.Errorf("migration %s needs an up and a down file, has %v", v, files[v])
		}
	}
	if got := files["0009"]["up"]; got != "0009_outbox_prepare.up.sql" {
		t.Errorf("0009 up = %q, want 0009_outbox_prepare.up.sql", got)
	}
	if got := files["0018"]["up"]; got != "0018_outbox_drop_claim_idx.up.sql" {
		t.Errorf("0018 up = %q, want 0018_outbox_drop_claim_idx.up.sql", got)
	}
	if got := files["0019"]["up"]; got != "0019_outbox_requeue_and_unify.up.sql" {
		t.Errorf("0019 up = %q, want 0019_outbox_requeue_and_unify.up.sql", got)
	}
}

// Scenario: Migrations 0001 to 0008 keep their down steps
func TestMigrations0001To0008KeepDownSteps(t *testing.T) {
	want := map[string]string{
		"0001": "users", "0002": "addresses", "0003": "password_reset_tokens", "0004": "sessions",
		"0005": "identity_outbox_events", "0006": "session_refresh", "0007": "user_status", "0008": "session_revoked_at",
	}
	files := migrationFiles(t)
	for ver, name := range want {
		for _, dir := range []string{"up", "down"} {
			file := files[ver][dir]
			if file != ver+"_"+name+"."+dir+".sql" {
				t.Errorf("migration %s %s = %q, want %s_%s.%s.sql", ver, dir, file, ver, name, dir)
				continue
			}
			b, err := fs.ReadFile(store.Migrations(), file)
			if err != nil || len(statementsOf(string(b))) == 0 {
				t.Errorf("%s must hold at least one statement (err %v)", file, err)
			}
		}
	}
}

func TestOutboxDownMigrationsRestoreTheOldShape(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	files := migrationFiles(t)
	for v := latestMigration; v >= 9; v-- {
		name := files[pad4(v)]["down"]
		b, err := fs.ReadFile(store.Migrations(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("down %s: %v", name, err)
		}
	}
	for _, col := range []string{"seq", "topic", "traceparent"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, outboxTable, col).Scan(&n); err != nil || n != 0 {
			t.Errorf("column %s after the downs: count %d, err %v, want 0", col, n, err)
		}
	}
	for _, ix := range []string{"identity_outbox_events_status_idx", "identity_outbox_events_agg_idx", "identity_outbox_events_pending_idx"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, ix).Scan(&n); err != nil || n != 0 {
			t.Errorf("index %s after the downs: count %d, err %v, want 0", ix, n, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'identity_outbox_claim_idx'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("identity_outbox_claim_idx after the downs: count %d, err %v, want 1", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relkind = 'S' AND relname = 'identity_outbox_events_seq_seq' AND relnamespace = current_schema()::regnamespace`).Scan(&n); err != nil || n != 0 {
		t.Errorf("sequence after the downs: count %d, err %v, want 0", n, err)
	}
}

func pad4(v int) string { return fmt.Sprintf("%04d", v) }

func seedV8(t *testing.T, pool *pgxpool.Pool, id, aggType, eventType, status string, age time.Duration) {
	t.Helper()
	attempts, errText := 0, (*string)(nil)
	var publishedAt *time.Time
	switch status {
	case "failed":
		attempts = 10
		msg := "kafka: broker unavailable"
		errText = &msg
	case "published":
		at := time.Now().Add(-age)
		publishedAt = &at
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO identity_outbox_events
		  (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, error, published_at, available_at, created_at)
		VALUES ($1, $2, 'u-ob1', $3, $4, $5, $6, $7, $8, now() - ($9 * interval '1 second'), now() - ($9 * interval '1 second'))`,
		id, aggType, eventType, []byte(id), status, attempts, errText, publishedAt, age.Seconds())
	if err != nil {
		t.Fatalf("seed v8 row %s: %v", id, err)
	}
}

// Scenario: The outbox upgrade keeps pending events in order
func TestOutboxUpgradeKeepsPendingOrder(t *testing.T) {
	dsn := schemaDSN(t)
	migrateTo(t, dsn, 8)
	pool := openPool(t, dsn)
	seedV8(t, pool, "evt-a", "Session", evSessionRevoked, "pending", 3*time.Hour)
	seedV8(t, pool, "evt-b", "Session", evSessionRevoked, "pending", 2*time.Hour)
	seedV8(t, pool, "evt-c", "User", evUserStatus, "pending", 1*time.Hour)

	migrateTo(t, dsn, latestMigration)
	enqueueUserRow(t, pool, outbox.Row{EventID: "evt-d", AggregateType: "User", AggregateID: "u-ob1", EventType: evUserStatus, Payload: []byte("evt-d")})

	got := relayValues(t, pool, 4)
	if want := "evt-a,evt-b,evt-c,evt-d"; strings.Join(got, ",") != want {
		t.Fatalf("relay order = %v, want %s", got, want)
	}
}

// Scenario: A row parked by the old relayer is relayed after the upgrade
func TestOutboxUpgradeRequeuesParkedRows(t *testing.T) {
	ctx := context.Background()
	dsn := schemaDSN(t)
	migrateTo(t, dsn, 8)
	pool := openPool(t, dsn)
	seedV8(t, pool, "evt-parked", "Session", evSessionRevoked, "failed", 2*time.Hour)
	seedV8(t, pool, "evt-next", "User", evUserStatus, "pending", 1*time.Hour)

	migrateTo(t, dsn, latestMigration)

	var status string
	var attempts int
	var errText *string
	if err := pool.QueryRow(ctx, `SELECT status, attempts, error FROM identity_outbox_events WHERE event_id = 'evt-parked'`).Scan(&status, &attempts, &errText); err != nil {
		t.Fatalf("read parked row: %v", err)
	}
	if status != "pending" || attempts != 0 || errText != nil {
		t.Fatalf("parked row: status=%s attempts=%d error=%v, want pending 0 nil", status, attempts, errText)
	}
	got := relayValues(t, pool, 2)
	if want := "evt-parked,evt-next"; strings.Join(got, ",") != want {
		t.Fatalf("relay order = %v, want %s", got, want)
	}
}

// Scenario: Unpublished rows of the old aggregate join the user aggregate
func TestOutboxUpgradeUnifiesAggregateType(t *testing.T) {
	ctx := context.Background()
	dsn := schemaDSN(t)
	migrateTo(t, dsn, 8)
	pool := openPool(t, dsn)
	seedV8(t, pool, "evt-old-pending", "Session", evSessionRevoked, "pending", 2*time.Hour)
	seedV8(t, pool, "evt-old-published", "Session", evSessionRevoked, "published", 3*time.Hour)

	migrateTo(t, dsn, latestMigration)
	enqueueUserRow(t, pool, outbox.Row{EventID: "evt-new", AggregateType: "User", AggregateID: "u-ob1", EventType: evUserStatus, Payload: []byte("evt-new")})

	types := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT event_id, aggregate_type FROM identity_outbox_events`)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, typ string
		if err := rows.Scan(&id, &typ); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types[id] = typ
	}
	rows.Close()
	if types["evt-old-pending"] != "User" {
		t.Errorf("pending row aggregate_type = %q, want User", types["evt-old-pending"])
	}
	if types["evt-old-published"] != "Session" {
		t.Errorf("published row aggregate_type = %q, want Session unchanged", types["evt-old-published"])
	}
	got := relayValues(t, pool, 2)
	if want := "evt-old-pending,evt-new"; strings.Join(got, ",") != want {
		t.Fatalf("relay order = %v, want %s", got, want)
	}
}
