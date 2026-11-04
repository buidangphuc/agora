package store_test

import (
	"io/fs"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func TestOutboxMigrationMatchesSDKSchema(t *testing.T) {
	want, err := outbox.SchemaSQL(outboxTable)
	if err != nil {
		t.Fatalf("SchemaSQL: %v", err)
	}
	if got := migrationSQL(t, "0005_outbox.up.sql"); got != want {
		t.Fatalf("0005_outbox.up.sql differs from outbox.SchemaSQL(%q):\n--- got\n%s\n--- want\n%s", outboxTable, got, want)
	}
}

func TestMigrateFromVersion4AppliesOutboxOnly(t *testing.T) {
	dsn := schemaDSN(t)
	pool := openPool(t, dsn)
	ctx := t.Context()

	for _, name := range []string{
		"0001_promotion.up.sql", "0002_subscriptions.up.sql", "0003_sponsored.up.sql", "0004_flash_sale_holds.up.sql",
	} {
		if _, err := pool.Exec(ctx, migrationSQL(t, name)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL);
		INSERT INTO schema_migrations (version, dirty) VALUES (4, false)`); err != nil {
		t.Fatalf("mark version 4: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code, scope, quota, used) VALUES ('v-old', 'OLD10', 2, 5, 1);
		INSERT INTO flash_sale_campaigns (id, listing_id, sale_price, stock_cap, stock_sold) VALUES ('c-old', 'l-old', 99, 10, 4)`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	if err := migrate.Up(ctx, dsn, migrationsFS(t)); err != nil {
		t.Fatalf("migrate up from version 4: %v", err)
	}

	var version int64
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 7 || dirty {
		t.Fatalf("schema_migrations = version %d dirty %v, want 7 false", version, dirty)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('`+outboxTable+`') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("%s exists=%v err=%v, want true", outboxTable, exists, err)
	}
	var code string
	var used int64
	if err := pool.QueryRow(ctx, `SELECT code, used FROM vouchers WHERE id = 'v-old'`).Scan(&code, &used); err != nil || code != "OLD10" || used != 1 {
		t.Fatalf("voucher after migrate = %q used %d err=%v, want unchanged", code, used, err)
	}
	var sold int64
	if err := pool.QueryRow(ctx, `SELECT stock_sold FROM flash_sale_campaigns WHERE id = 'c-old'`).Scan(&sold); err != nil || sold != 4 {
		t.Fatalf("campaign stock_sold = %d err=%v, want 4", sold, err)
	}
	if n := outboxCount(t, pool); n != 0 {
		t.Fatalf("outbox rows after migrate = %d, want 0", n)
	}

	if _, err := pool.Exec(ctx, migrationSQL(t, "0005_outbox.down.sql")); err != nil {
		t.Fatalf("apply 0005 down: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('`+outboxTable+`') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("%s exists=%v err=%v after down, want false", outboxTable, exists, err)
	}
}

func TestMigration0007CampaignCancelAndIdempotency_CancelledAtColumn(t *testing.T) {
	dsn := schemaDSN(t)
	pool := openPool(t, dsn)
	ctx := t.Context()
	ups, err := fs.Glob(store.Migrations, "migrations/0007_*.up.sql")
	if err != nil || len(ups) != 1 {
		t.Fatalf("0007 up migration = %v err=%v, want exactly one", ups, err)
	}
	downs, err := fs.Glob(store.Migrations, "migrations/0007_*.down.sql")
	if err != nil || len(downs) != 1 {
		t.Fatalf("0007 down migration = %v err=%v, want exactly one", downs, err)
	}
	if err := migrate.Up(ctx, dsn, migrationsFS(t)); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	hasColumn := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'flash_sale_campaigns' AND column_name = 'cancelled_at')`).Scan(&ok); err != nil {
			t.Fatalf("inspect columns: %v", err)
		}
		return ok
	}
	if !hasColumn() {
		t.Fatal("flash_sale_campaigns.cancelled_at missing after migrate up")
	}
	var nulls int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM flash_sale_campaigns WHERE cancelled_at IS NOT NULL`).Scan(&nulls); err != nil || nulls != 0 {
		t.Fatalf("cancelled rows on fresh table = %d err=%v", nulls, err)
	}
	body, err := fs.ReadFile(store.Migrations, downs[0])
	if err != nil {
		t.Fatalf("read down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply 0007 down: %v", err)
	}
	if hasColumn() {
		t.Fatal("cancelled_at still present after 0007 down")
	}
}
