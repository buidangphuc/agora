package migrations_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/migrations"
)

const bundleLinesUp = "0025_bundle_lines.up.sql"

func newMigDB0025(t *testing.T) *mdb {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= "0025" {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		before[e.Name()] = &fstest.MapFile{Data: b}
	}
	if err := migrate.Up(t.Context(), dsn, before); err != nil {
		t.Fatalf("migrate to 0024: %v", err)
	}
	return &mdb{t: t, ctx: t.Context(), pool: pool}
}

func (m *mdb) runBundleUp() {
	m.t.Helper()
	b, err := os.ReadFile(bundleLinesUp)
	if err != nil {
		m.t.Fatalf("read %s: %v", bundleLinesUp, err)
	}
	if _, err := m.pool.Exec(m.ctx, string(b)); err != nil {
		m.t.Fatalf("apply %s: %v", bundleLinesUp, err)
	}
}

// Scenario (order-bundle-checkout): Re-running the migration changes nothing.
func TestBundleLinesMigrationIdempotent(t *testing.T) {
	m := newMigDB0025(t)
	m.exec(`INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, seller_id)
		VALUES ('c1','u1','l1','',2,500,'t','s1'), ('c2','u1','l2','v',1,700,'t','s1')`)
	m.order("o1", stPending, mCOD, false)
	m.line("o1", "l1", 2)

	m.runBundleUp()
	m.runBundleUp()

	if n := m.count(`SELECT count(*) FROM cart_items WHERE bundle_id <> '' OR bundle_discount <> 0`); n != 0 {
		t.Fatalf("cart rows with bundle data after rerun = %d, want 0", n)
	}
	if n := m.count(`SELECT count(*) FROM order_items WHERE bundle_id <> '' OR bundle_discount <> 0`); n != 0 {
		t.Fatalf("order rows with bundle data after rerun = %d, want 0", n)
	}
	if n := m.count(`SELECT count(*) FROM cart_items`); n != 2 {
		t.Fatalf("cart rows = %d, want 2", n)
	}
	if n := m.count(`SELECT count(*) FROM order_items`); n != 1 {
		t.Fatalf("order rows = %d, want 1", n)
	}
}

// Scenario (order-bundle-checkout): A bundle line stays separate from a plain line of the same listing.
func TestBundleLinesMigrationKeyAllowsPlainAndBundleLine(t *testing.T) {
	m := newMigDB0025(t)
	m.runBundleUp()
	m.exec(`INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, seller_id)
		VALUES ('p','u1','l1','',1,500,'t','s1')`)
	m.exec(`INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, seller_id, bundle_id, bundle_discount)
		VALUES ('b','u1','l1','',1,500,'t','s1','B1',50)`)
	if _, err := m.pool.Exec(m.ctx, `INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, seller_id, bundle_id, bundle_discount)
		VALUES ('b2','u1','l1','',1,500,'t','s1','B1',50)`); err == nil {
		t.Fatal("duplicate (user, listing, variant, bundle) accepted")
	}
	if n := m.count(`SELECT count(*) FROM cart_items WHERE user_id='u1'`); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
}

// Scenario (order-bundle-checkout): A plain cart is priced as before (legacy rows default to no bundle).
func TestBundleLinesMigrationLegacyRowsPlain(t *testing.T) {
	m := newMigDB0025(t)
	m.exec(`INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, seller_id)
		VALUES ('c1','u1','l1','',2,500,'t','s1')`)
	m.runBundleUp()
	if n := m.count(`SELECT count(*) FROM cart_items WHERE bundle_id = '' AND bundle_discount = 0`); n != 1 {
		t.Fatalf("legacy row not plain after migration")
	}
}
