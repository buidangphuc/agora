package migrations_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

var fileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.(up|down)\.sql$`)

func listMigrations(t *testing.T) map[string]map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	out := map[string]map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("migration %s does not match NNNN_name.(up|down).sql", e.Name())
			continue
		}
		if out[m[1]] == nil {
			out[m[1]] = map[string]string{}
		}
		out[m[1]][m[2]] = e.Name()
	}
	return out
}

func normalise(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		kept = append(kept, line)
	}
	s = strings.Join(kept, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ";"))
}

func TestConcurrentIndexMigrationsAreSingleStatement(t *testing.T) {
	up, err := outbox.UpgradeSQL("chat_outbox_events")
	if err != nil {
		t.Fatalf("upgrade sql: %v", err)
	}
	finish := up.Finish
	want := map[string]string{"0008": finish[5], "0009": finish[6], "0010": finish[7]}
	files := listMigrations(t)
	for _, ver := range []string{"0008", "0009", "0010"} {
		name := files[ver]["up"]
		if name == "" {
			t.Errorf("migration %s up file is missing", ver)
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := normalise(string(b))
		upper := strings.ToUpper(body)
		if strings.Contains(upper, "BEGIN") || strings.Contains(upper, "COMMIT") {
			t.Errorf("%s must not open a transaction", name)
		}
		if strings.Contains(body, ";") {
			t.Errorf("%s must hold exactly one statement", name)
		}
		if !strings.HasPrefix(upper, "CREATE INDEX CONCURRENTLY IF NOT EXISTS ") {
			t.Errorf("%s must be CREATE INDEX CONCURRENTLY IF NOT EXISTS, got %q", name, body)
		}
		if body != normalise(want[ver]) {
			t.Errorf("%s differs from the SDK statement:\n got %q\nwant %q", name, body, normalise(want[ver]))
		}
	}
}

func TestEveryUpHasDown(t *testing.T) {
	files := listMigrations(t)
	var versions []string
	for v := range files {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	if len(versions) < 12 || versions[len(versions)-1] != "0012" {
		t.Fatalf("expected migrations 0001..0012, found %v", versions)
	}
	for i, v := range versions {
		if v != fmt.Sprintf("%04d", i+1) {
			t.Fatalf("migration versions are not contiguous: %v", versions)
		}
		if files[v]["up"] == "" || files[v]["down"] == "" {
			t.Errorf("migration %s needs both up and down, has %v", v, files[v])
		}
	}
	if t.Failed() {
		return
	}

	ctx := context.Background()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(ctx, url, os.DirFS(".")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := postgres.Open(ctx, config.Postgres{URL: url, MaxConns: 2})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()
	for i := len(versions) - 1; i >= 3; i-- {
		b, err := os.ReadFile(files[versions[i]]["down"])
		if err != nil {
			t.Fatalf("read down %s: %v", versions[i], err)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("down %s: %v", versions[i], err)
		}
	}
	for _, col := range []string{"topic", "traceparent", "seq"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'chat_outbox_events' AND column_name = $1`, col).Scan(&n); err != nil {
			t.Fatalf("query column %s: %v", col, err)
		}
		if n != 0 {
			t.Errorf("column %s still present after the down migrations", col)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'chat_outbox_events'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("v0 table must remain after downs to 0003 (n=%d err=%v)", n, err)
	}
}
