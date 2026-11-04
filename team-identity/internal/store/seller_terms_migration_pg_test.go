package store_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

const sellerTermsMigration = 21

// Scenario: Seller onboarding: acceptance is recorded
func TestUsersTableHasSellerTermsColumns(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	for col, typ := range map[string]string{
		"seller_terms_version":     "text",
		"seller_terms_accepted_at": "timestamp with time zone",
	} {
		var gotType, nullable string
		err := pool.QueryRow(ctx, `SELECT data_type, is_nullable FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = $1`, col).Scan(&gotType, &nullable)
		if err != nil {
			t.Fatalf("column users.%s: %v", col, err)
		}
		if gotType != typ || nullable != "YES" {
			t.Errorf("users.%s = %s nullable=%s, want %s nullable=YES", col, gotType, nullable, typ)
		}
	}
}

// Scenario: Seller onboarding: acceptance is recorded
func TestSellerTermsColumnsStoreVersionAndTime(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	at := time.Date(2026, 11, 4, 9, 30, 15, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash, seller_terms_version, seller_terms_accepted_at)
		VALUES ('u-st1','u-st1','x',$1,$2)`, "V1", at); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var v string
	var got time.Time
	if err := pool.QueryRow(ctx, `SELECT seller_terms_version, seller_terms_accepted_at FROM users WHERE id='u-st1'`).Scan(&v, &got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if v != "V1" || got.Sub(at).Abs() > time.Second {
		t.Fatalf("stored (%q, %v), want (V1, %v)", v, got, at)
	}
}

// Scenario: Seller onboarding: acceptance is recorded
func TestSellerTermsColumnsDefaultNullForExistingUsers(t *testing.T) {
	ctx := context.Background()
	dsn := schemaDSN(t)
	migrateTo(t, dsn, sellerTermsMigration-1)
	pool := openPool(t, dsn)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, password_hash) VALUES ('u-st0','u-st0','x')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	migrateTo(t, dsn, sellerTermsMigration)
	var v *string
	var at *time.Time
	if err := pool.QueryRow(ctx, `SELECT seller_terms_version, seller_terms_accepted_at FROM users WHERE id='u-st0'`).Scan(&v, &at); err != nil {
		t.Fatalf("select after migration: %v", err)
	}
	if v != nil || at != nil {
		t.Fatalf("existing user got (%v, %v), want NULL, NULL", v, at)
	}
}

// Scenario: Seller onboarding: acceptance is recorded
func TestSellerTermsDownMigrationDropsColumns(t *testing.T) {
	ctx := context.Background()
	pool := latestPool(t)
	b, err := fs.ReadFile(store.Migrations(), migrationFiles(t)[pad4(sellerTermsMigration)]["down"])
	if err != nil {
		t.Fatalf("read down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(b)); err != nil {
		t.Fatalf("down: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'users' AND column_name LIKE 'seller_terms_%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("seller_terms columns after down: count %d, err %v, want 0", n, err)
	}
}
