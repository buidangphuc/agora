package store_test

import (
	"context"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const outboxTable = "chat_outbox_events"

func migrationVersion(name string) int {
	head, _, ok := strings.Cut(name, "_")
	if !ok {
		return -1
	}
	n, err := strconv.Atoi(head)
	if err != nil {
		return -1
	}
	return n
}

func migrationsUpTo(t *testing.T, maxVer int) fs.FS {
	t.Helper()
	dir := os.DirFS("../../migrations")
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	out := fstest.MapFS{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v := migrationVersion(e.Name())
		if v < 0 || v > maxVer {
			continue
		}
		b, err := fs.ReadFile(dir, e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

func freshSchema(t *testing.T) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func migrateTo(t *testing.T, url string, maxVer int) {
	t.Helper()
	if err := migrate.Up(context.Background(), url, migrationsUpTo(t, maxVer)); err != nil {
		t.Fatalf("migrate up to %04d: %v", maxVer, err)
	}
}

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedV0(t *testing.T, pool *pgxpool.Pool, id, status string, age time.Duration) {
	t.Helper()
	attempts, errText := 0, (*string)(nil)
	if status == "failed" {
		attempts = 10
		msg := "kafka: broker unavailable"
		errText = &msg
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO chat_outbox_events
		  (event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, error, created_at)
		VALUES ($1, 'ChatThread', 'thread-1', 'platform.chat.v1.ChatMessage', $2, $3, $4, $5, now() - ($6 * interval '1 second'))`,
		id, []byte(id), status, attempts, errText, age.Seconds())
	if err != nil {
		t.Fatalf("seed v0 row %s: %v", id, err)
	}
}

func seqOf(t *testing.T, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var seq *int64
	if err := pool.QueryRow(context.Background(), `SELECT seq FROM chat_outbox_events WHERE event_id = $1`, id).Scan(&seq); err != nil {
		t.Fatalf("read seq of %s: %v", id, err)
	}
	if seq == nil {
		t.Fatalf("row %s has no seq after the upgrade", id)
	}
	return *seq
}

func relayAll(t *testing.T, pool *pgxpool.Pool, want int) []string {
	t.Helper()
	store, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(store, rec, outbox.RelayerOptions{
		Topic:  "chat.events",
		Config: config.Outbox{BatchSize: 10, ClaimLock: time.Minute},
	})
	if err != nil {
		t.Fatalf("relayer: %v", err)
	}
	for i := 0; i < 10 && len(rec.Records()) < want; i++ {
		if _, err := relayer.RelayOnce(context.Background()); err != nil {
			t.Fatalf("relay once: %v", err)
		}
	}
	var got []string
	for _, r := range rec.Records() {
		got = append(got, string(r.Value))
	}
	return got
}

func TestOutboxUpgradeKeepsRowOrder(t *testing.T) {
	url := freshSchema(t)
	migrateTo(t, url, 3)
	pool := openPool(t, url)
	seedV0(t, pool, "evt-c", "pending", 3*time.Hour)
	seedV0(t, pool, "evt-a", "pending", 2*time.Hour)
	seedV0(t, pool, "evt-b", "pending", 1*time.Hour)

	migrateTo(t, url, 12)

	c, a, b := seqOf(t, pool, "evt-c"), seqOf(t, pool, "evt-a"), seqOf(t, pool, "evt-b")
	if c >= a || a >= b {
		t.Fatalf("seq must follow created_at order, got c=%d a=%d b=%d", c, a, b)
	}
	got := relayAll(t, pool, 3)
	want := []string{"evt-c", "evt-a", "evt-b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("relay order = %v, want %v", got, want)
	}
}

func TestOutboxBackfillIsBounded(t *testing.T) {
	url := freshSchema(t)
	migrateTo(t, url, 3)
	pool := openPool(t, url)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO chat_outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, created_at)
		SELECT 'evt-' || g, 'ChatThread', 'thread-' || (g % 50), 'platform.chat.v1.ChatMessage', 'x'::bytea,
		       now() - (g * interval '1 second')
		FROM generate_series(1, 10000) g`)
	if err != nil {
		t.Fatalf("seed 10000 rows: %v", err)
	}
	migrateTo(t, url, 4)

	start := time.Now()
	migrateTo(t, url, 5)
	if took := time.Since(start); took >= 5*time.Second {
		t.Fatalf("migration 0005 took %s, want under 5s", took)
	}

	migrateTo(t, url, 12)
	var total, distinct, nulls int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(DISTINCT seq), count(*) FILTER (WHERE seq IS NULL) FROM chat_outbox_events`).
		Scan(&total, &distinct, &nulls); err != nil {
		t.Fatalf("count seq: %v", err)
	}
	if total != 10000 || distinct != 10000 || nulls != 0 {
		t.Fatalf("total=%d distinct=%d nulls=%d, want 10000 10000 0", total, distinct, nulls)
	}
}

var (
	createTableBody = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS \S+ \((.*?)\n\);`)
	createIndexName = regexp.MustCompile(`CREATE INDEX IF NOT EXISTS (\S+)`)
)

func TestOutboxSchemaMatchesSDK(t *testing.T) {
	ctx := context.Background()
	url := freshSchema(t)
	migrateTo(t, url, 12)
	pool := openPool(t, url)

	sdk, err := outbox.SchemaSQL(outboxTable)
	if err != nil {
		t.Fatalf("sdk schema: %v", err)
	}
	m := createTableBody.FindStringSubmatch(sdk)
	if m == nil {
		t.Fatalf("cannot parse sdk schema")
	}
	var cols []string
	for _, line := range strings.Split(m[1], "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] != "PRIMARY" {
			cols = append(cols, f[0])
		}
	}
	if len(cols) < 14 {
		t.Fatalf("parsed too few sdk columns: %v", cols)
	}
	for _, c := range cols {
		var nullable string
		err := pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, outboxTable, c).Scan(&nullable)
		if err != nil {
			t.Errorf("column %s missing: %v", c, err)
			continue
		}
		if c == "seq" && nullable != "NO" {
			t.Errorf("seq must be NOT NULL")
		}
	}

	indexes := createIndexName.FindAllStringSubmatch(sdk, -1)
	if len(indexes) != 3 {
		t.Fatalf("expected 3 sdk indexes, parsed %d", len(indexes))
	}
	for _, ix := range indexes {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, ix[1]).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s missing (n=%d err=%v)", ix[1], n, err)
		}
	}
	for _, old := range []string{"chat_outbox_events_claim_idx", "idx_chat_outbox_claim"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, old).Scan(&n); err != nil {
			t.Fatalf("query index %s: %v", old, err)
		}
		if n != 0 {
			t.Errorf("v0 claim index %s must be dropped", old)
		}
	}

	var def *string
	if err := pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'seq'`, outboxTable).Scan(&def); err != nil {
		t.Fatalf("seq default: %v", err)
	}
	if def == nil || !strings.Contains(*def, "nextval") {
		t.Errorf("seq must default to a sequence, got %v", def)
	}
}

func TestOutboxUpgradeRequeuesFailedRows(t *testing.T) {
	ctx := context.Background()
	url := freshSchema(t)
	migrateTo(t, url, 3)
	pool := openPool(t, url)
	seedV0(t, pool, "evt-1", "failed", 3*time.Hour)
	seedV0(t, pool, "evt-2", "failed", 2*time.Hour)
	seedV0(t, pool, "evt-3", "pending", 1*time.Hour)

	migrateTo(t, url, 12)

	rows, err := pool.Query(ctx, `SELECT event_id, status, attempts, error FROM chat_outbox_events ORDER BY seq`)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id, status string
		var attempts int
		var errText *string
		if err := rows.Scan(&id, &status, &attempts, &errText); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if status != "pending" || attempts != 0 || errText != nil {
			t.Errorf("row %s: status=%s attempts=%d error=%v, want pending 0 nil", id, status, attempts, errText)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if strings.Join(ids, ",") != "evt-1,evt-2,evt-3" {
		t.Fatalf("rows in seq order = %v", ids)
	}
	got := relayAll(t, pool, 3)
	if strings.Join(got, ",") != "evt-1,evt-2,evt-3" {
		t.Fatalf("relay order = %v, want evt-1,evt-2,evt-3", got)
	}
}
