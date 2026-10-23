package store_test

import (
	"context"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-audit/internal/audit"
	"github.com/buidangphuc/team-audit/internal/store"
	"github.com/buidangphuc/team-audit/internal/store/storetest"
)

var (
	_ audit.Repository = (*store.Postgres)(nil)
	_ audit.Repository = (*storetest.Memory)(nil)
)

func at(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

var filterSeed = []audit.Event{
	{ID: "e1", ActorID: "adm-1", Action: "kyc.review", TargetType: "order", TargetID: "o-1", CreatedAt: at(10)},
	{ID: "e2", ActorID: "adm-1", Action: "kyc.reviewed", TargetType: "order", TargetID: "o-1", CreatedAt: at(11)},
	{ID: "e3", ActorID: "adm-2", Action: "review.hide", TargetType: "order", TargetID: "o-2", CreatedAt: at(12)},
	{ID: "e4", ActorID: "adm-1", Action: "order.force_fail", TargetType: "order", TargetID: "o-1", CreatedAt: at(12)},
	{ID: "e5", ActorID: "adm-1", Action: "kyc.review", TargetType: "listing", TargetID: "l-1", CreatedAt: at(13)},
}

type filterCase struct {
	name      string
	f         audit.Filter
	wantIDs   []string
	wantTotal int64
}

var filterCases = []filterCase{
	{"no filter", audit.Filter{}, []string{"e5", "e4", "e3", "e2", "e1"}, 5},
	{"target type and id narrow to one target",
		audit.Filter{TargetType: "order", TargetID: "o-1"}, []string{"e4", "e2", "e1"}, 3},
	{"target id alone", audit.Filter{TargetID: "o-2"}, []string{"e3"}, 1},
	{"action exact match excludes prefix and suffix variants",
		audit.Filter{Action: "kyc.review"}, []string{"e5", "e1"}, 2},
	{"filters combine with AND",
		audit.Filter{ActorID: "adm-1", Action: "order.force_fail", TargetID: "o-1"}, []string{"e4"}, 1},
	{"AND leaves nothing when one filter misses",
		audit.Filter{ActorID: "adm-2", Action: "order.force_fail", TargetID: "o-1"}, nil, 0},
	{"lower bound inclusive, upper bound exclusive",
		audit.Filter{CreatedAfter: at(10), CreatedBefore: at(12)}, []string{"e2", "e1"}, 2},
	{"only created_after", audit.Filter{CreatedAfter: at(12)}, []string{"e5", "e4", "e3"}, 3},
	{"only created_before is exclusive", audit.Filter{CreatedBefore: at(11)}, []string{"e1"}, 1},
	{"equal bounds match nothing", audit.Filter{CreatedAfter: at(11), CreatedBefore: at(11)}, nil, 0},
	{"time range combined with action",
		audit.Filter{Action: "kyc.review", CreatedAfter: at(11)}, []string{"e5"}, 1},
	{"filter values are bound parameters (action)",
		audit.Filter{Action: "x' OR '1'='1"}, nil, 0},
	{"filter values are bound parameters (target id)",
		audit.Filter{TargetID: "o-1' OR '1'='1"}, nil, 0},
	{"no match", audit.Filter{TargetID: "nope"}, nil, 0},
	{"total ignores limit and offset",
		audit.Filter{Action: "kyc.review", Limit: 1, Offset: 1}, []string{"e1"}, 2},
	{"filtered second page",
		audit.Filter{TargetType: "order", TargetID: "o-1", Limit: 2, Offset: 2}, []string{"e1"}, 3},
}

func runFilterCases(t *testing.T, repo audit.Repository) {
	t.Helper()
	ctx := context.Background()
	for _, c := range filterCases {
		t.Run(c.name, func(t *testing.T) {
			evs, total, err := repo.Query(ctx, c.f)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			got := []string{}
			for _, e := range evs {
				got = append(got, e.ID)
			}
			want := c.wantIDs
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ids = %v, want %v", got, want)
			}
			if total != c.wantTotal {
				t.Fatalf("total = %d, want %d", total, c.wantTotal)
			}
		})
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

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Postgres(t)
	if err := migrate.Up(context.Background(), url, store.Migrations()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return openPool(t, url)
}

func seedPostgres(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, ev := range filterSeed {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO audit_events (id, actor_id, action, target_type, target_id, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			ev.ID, ev.ActorID, ev.Action, ev.TargetType, ev.TargetID, ev.CreatedAt); err != nil {
			t.Fatalf("seed %s: %v", ev.ID, err)
		}
	}
}

func seedMemory(t *testing.T, m *storetest.Memory) {
	t.Helper()
	for _, ev := range filterSeed {
		ev.Metadata = map[string]string{}
		if _, err := m.Append(context.Background(), ev); err != nil {
			t.Fatalf("seed %s: %v", ev.ID, err)
		}
	}
}

func TestRepositoryFilterInMemory(t *testing.T) {
	m := storetest.NewMemory()
	seedMemory(t, m)
	runFilterCases(t, m)
}

func TestRepositoryFilterParity(t *testing.T) {
	pool := migratedPool(t)
	seedPostgres(t, pool)
	runFilterCases(t, store.NewPostgres(pool))
}

func TestQueryFilterValueIsBound(t *testing.T) {
	ctx := context.Background()
	pool := migratedPool(t)
	seedPostgres(t, pool)
	repo := store.NewPostgres(pool)

	evs, total, err := repo.Query(ctx, audit.Filter{Action: "x' OR '1'='1"})
	if err != nil {
		t.Fatalf("hostile query errored: %v", err)
	}
	if len(evs) != 0 || total != 0 {
		t.Fatalf("hostile action matched %d rows (total %d), want none", len(evs), total)
	}
	evs, total, err = repo.Query(ctx, audit.Filter{ActorID: "'; DROP TABLE audit_events; --"})
	if err != nil || len(evs) != 0 || total != 0 {
		t.Fatalf("hostile actor: evs=%d total=%d err=%v", len(evs), total, err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&rows); err != nil || rows != len(filterSeed) {
		t.Fatalf("stored rows = %d err=%v, want %d intact", rows, err, len(filterSeed))
	}
}

func actionIndexCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes
		 WHERE schemaname = current_schema() AND indexname = 'idx_audit_events_action_created'`).Scan(&n)
	if err != nil {
		t.Fatalf("count index: %v", err)
	}
	return n
}

func execFile(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	sql, err := fs.ReadFile(store.Migrations(), name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func TestMigrationFilesAreFourDigit(t *testing.T) {
	entries, err := fs.ReadDir(store.Migrations(), ".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{
		"0001_create_audit_events.down.sql", "0001_create_audit_events.up.sql",
		"0002_audit_action_created_index.down.sql", "0002_audit_action_created_index.up.sql",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("migrations = %v, want %v", names, want)
	}
}

func TestMigration002ActionIndex(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	pool := openPool(t, url)
	execFile(t, pool, "0001_create_audit_events.up.sql")
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events (id, action) VALUES ('keep-1', 'a.b')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n := actionIndexCount(t, pool); n != 0 {
		t.Fatalf("index must not exist before 0002, got %d", n)
	}

	execFile(t, pool, "0002_audit_action_created_index.up.sql")
	if n := actionIndexCount(t, pool); n != 1 {
		t.Fatalf("after up: want 1 index, got %d", n)
	}
	var def string
	if err := pool.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema()
		 AND indexname = 'idx_audit_events_action_created'`).Scan(&def); err != nil {
		t.Fatalf("indexdef: %v", err)
	}
	for _, want := range []string{"audit_events", "action", "created_at DESC"} {
		if !strings.Contains(def, want) {
			t.Fatalf("indexdef %q lacks %q", def, want)
		}
	}

	execFile(t, pool, "0002_audit_action_created_index.up.sql")
	if n := actionIndexCount(t, pool); n != 1 {
		t.Fatalf("after second up: want exactly 1 index, got %d", n)
	}

	execFile(t, pool, "0002_audit_action_created_index.down.sql")
	if n := actionIndexCount(t, pool); n != 0 {
		t.Fatalf("after down: want 0 indexes, got %d", n)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("existing rows must be untouched: rows=%d err=%v", rows, err)
	}
}

func TestMigrateUpAppliesEmbeddedFiles(t *testing.T) {
	pool := migratedPool(t)
	if n := actionIndexCount(t, pool); n != 1 {
		t.Fatalf("migrate.Up on embedded files: want index, got %d", n)
	}
}
