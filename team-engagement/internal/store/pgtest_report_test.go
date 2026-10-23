package store_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-engagement/migrations"
)

// Scenario: Listing reports: Migration creates the schema (files present; needs no database)
func TestMigrationReportsFilesExist(t *testing.T) {
	for _, f := range []string{"0012_reports.up.sql", "0012_reports.down.sql"} {
		b, err := fs.ReadFile(migrations.FS, f)
		if err != nil || strings.TrimSpace(string(b)) == "" {
			t.Fatalf("migration %s missing or empty: %v", f, err)
		}
	}
}

func reportsIndexDefs(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT indexdef FROM pg_indexes WHERE tablename='reports'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var defs []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		defs = append(defs, strings.ToLower(d))
	}
	return defs
}

func hasIndex(defs []string, parts ...string) bool {
next:
	for _, d := range defs {
		for _, p := range parts {
			if !strings.Contains(d, p) {
				continue next
			}
		}
		return true
	}
	return false
}

// Scenario: Listing reports: Migration creates the schema
func TestMigrationReportsUp(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	for _, col := range []string{"id", "target_type", "target_id", "reason_code", "note", "reporter_id",
		"status", "created_at", "resolved_at", "resolved_by", "resolution_note"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_name='reports' AND column_name=$1`, col).Scan(&n); err != nil || n != 1 {
			t.Errorf("column reports.%s present=%d err=%v, want 1", col, n, err)
		}
	}

	defs := reportsIndexDefs(t, pool)
	if !hasIndex(defs, "unique", "(reporter_id, target_type, target_id)", "open") {
		t.Errorf("missing partial unique OPEN index; have %v", defs)
	}
	if !hasIndex(defs, "(status, created_at, id)") {
		t.Errorf("missing (status, created_at, id) index; have %v", defs)
	}
	if !hasIndex(defs, "(target_id)") {
		t.Errorf("missing (target_id) index; have %v", defs)
	}
	if !hasIndex(defs, "(resolved_at)") {
		t.Errorf("missing (resolved_at) index; have %v", defs)
	}

	// Behaviour: an OPEN duplicate is refused, a closed one is not.
	ins := `INSERT INTO reports (id, target_type, target_id, reason_code, note, reporter_id, status)
		VALUES ($1,'LISTING','L1','SPAM','','u1',$2)`
	if _, err := pool.Exec(ctx, ins, "r1", "OPEN"); err != nil {
		t.Fatalf("first open insert: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, "r2", "OPEN"); err == nil {
		t.Fatal("duplicate OPEN report accepted, want unique violation")
	}
	if _, err := pool.Exec(ctx, `UPDATE reports SET status='RESOLVED', resolved_at=now() WHERE id='r1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ins, "r3", "OPEN"); err != nil {
		t.Fatalf("new OPEN after resolve must be allowed: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, "r4", "RESOLVED"); err != nil {
		t.Fatalf("second non-OPEN row must be allowed: %v", err)
	}
}

// Scenario: Listing reports: Down migration removes the schema
func TestMigrationReportsDown(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO reviews (id, listing_id, user_id, user_name, order_id, rating, comment)
		VALUES ('keep-1','L1','u','U','',5,'')`); err != nil {
		t.Fatal(err)
	}
	var tablesBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()`).Scan(&tablesBefore)

	b, err := fs.ReadFile(migrations.FS, "0012_reports.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(b)); err != nil {
		t.Fatalf("down: %v", err)
	}

	var tbl, idx, tablesAfter, reviews int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name='reports'`).Scan(&tbl)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='reports'`).Scan(&idx)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()`).Scan(&tablesAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM reviews`).Scan(&reviews)
	if tbl != 0 || idx != 0 {
		t.Fatalf("after down: reports table=%d indexes=%d, want 0/0", tbl, idx)
	}
	if tablesAfter != tablesBefore-1 || reviews != 1 {
		t.Fatalf("other tables changed: tables %d->%d reviews=%d", tablesBefore, tablesAfter, reviews)
	}
}
