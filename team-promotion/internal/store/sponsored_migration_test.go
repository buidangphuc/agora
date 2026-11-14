package store_test

import (
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func readMigration0009(t *testing.T, suffix string) string {
	t.Helper()
	files, err := fs.Glob(store.Migrations, "migrations/0009_*."+suffix+".sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("0009 %s migration = %v err=%v, want exactly one", suffix, files, err)
	}
	body, err := fs.ReadFile(store.Migrations, files[0])
	if err != nil {
		t.Fatalf("read %s: %v", files[0], err)
	}
	return string(body)
}

func columnsOf(t *testing.T, pool *pgxpool.Pool, table string) map[string]bool {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1`, table)
	if err != nil {
		t.Fatalf("inspect %s: %v", table, err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[c] = true
	}
	return out
}

func TestMigrationEndsUnservableLegacyCampaigns(t *testing.T) {
	pool := newPoolUpTo(t, 8)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status) VALUES
		('zero-bid', 's1', 'l1', 5000, 0, 1),
		('bid-499', 's1', 'l2', 5000, 499, 1),
		('budget-short', 's1', 'l3', 700, 800, 1),
		('valid', 's1', 'l4', 5000, 800, 1),
		('valid-paused', 's1', 'l5', 5000, 800, 2)`); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}
	if _, err := pool.Exec(ctx, readMigration0009(t, "up")); err != nil {
		t.Fatalf("apply 0009 up: %v", err)
	}
	want := map[string]int32{"zero-bid": 3, "bid-499": 3, "budget-short": 3, "valid": 1, "valid-paused": 2}
	for id, st := range want {
		var got int32
		if err := pool.QueryRow(ctx, `SELECT status FROM ad_campaigns WHERE id = $1`, id).Scan(&got); err != nil || got != st {
			t.Fatalf("%s status = %d err=%v, want %d", id, got, err, st)
		}
	}
	var spent, impressions, clicks int64
	var category string
	if err := pool.QueryRow(ctx, `SELECT spent, impressions, clicks, category_id FROM ad_campaigns WHERE id = 'valid'`).Scan(&spent, &impressions, &clicks, &category); err != nil {
		t.Fatalf("read valid row: %v", err)
	}
	if spent != 0 || impressions != 0 || clicks != 0 || category != "" {
		t.Fatalf("valid row = spent %d impressions %d clicks %d category %q, want zeros and empty", spent, impressions, clicks, category)
	}
}

func TestMigrationDownRestoresSchema(t *testing.T) {
	pool := newPoolUpTo(t, 8)
	ctx := t.Context()
	before := columnsOf(t, pool, "ad_campaigns")
	if _, err := pool.Exec(ctx, readMigration0009(t, "up")); err != nil {
		t.Fatalf("apply 0009 up: %v", err)
	}
	after := columnsOf(t, pool, "ad_campaigns")
	for _, c := range []string{"spent", "impressions", "clicks", "category_id"} {
		if !after[c] {
			t.Fatalf("column %s missing after up", c)
		}
	}
	if len(columnsOf(t, pool, "ad_impressions")) == 0 {
		t.Fatal("ad_impressions missing after up")
	}
	if _, err := pool.Exec(ctx, readMigration0009(t, "down")); err != nil {
		t.Fatalf("apply 0009 down: %v", err)
	}
	restored := columnsOf(t, pool, "ad_campaigns")
	if len(restored) != len(before) {
		t.Fatalf("ad_campaigns columns after down = %v, want %v", restored, before)
	}
	for c := range before {
		if !restored[c] {
			t.Fatalf("pre-existing column %s lost", c)
		}
	}
	for _, c := range []string{"spent", "impressions", "clicks", "category_id"} {
		if restored[c] {
			t.Fatalf("column %s still present after down", c)
		}
	}
	if len(columnsOf(t, pool, "ad_impressions")) != 0 {
		t.Fatal("ad_impressions still present after down")
	}
}
