package consumer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-notification/internal/repository"
)

// pgBundle builds fresh Postgres-backed stores over the pool each call, like a
// restarted process re-opening its database.
func pgBundle(pool *pgxpool.Pool) stateBundle {
	return stateBundle{
		dedupe: repository.NewPostgresProcessedEventRepo(pool),
		price:  repository.NewPostgresListingPriceStore(pool),
		stock:  repository.NewPostgresListingStockStore(pool),
	}
}

func consumerStatePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres consumer-state test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	files, _ := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	sort.Strings(files)
	for _, f := range files {
		ddl, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	for _, q := range []string{`DELETE FROM processed_events`, `DELETE FROM listing_last_seen`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return pool
}

func TestPostgresDeduper(t *testing.T) {
	pool := consumerStatePool(t)
	ctx := context.Background()
	d := repository.NewPostgresProcessedEventRepo(pool)

	if ok, err := d.IsProcessed(ctx, "c1", "e1"); err != nil || ok {
		t.Fatalf("fresh id: processed=%v err=%v", ok, err)
	}
	if fresh, err := d.MarkProcessed(ctx, "c1", "e1"); err != nil || !fresh {
		t.Fatalf("first mark: fresh=%v err=%v", fresh, err)
	}
	if fresh, err := d.MarkProcessed(ctx, "c1", "e1"); err != nil || fresh {
		t.Fatalf("second mark must report not-new: fresh=%v err=%v", fresh, err)
	}
	if ok, _ := d.IsProcessed(ctx, "c1", "e1"); !ok {
		t.Fatal("marked id must be processed")
	}
	// The ledger is namespaced per consumer.
	if ok, _ := d.IsProcessed(ctx, "c2", "e1"); ok {
		t.Fatal("another consumer's ledger must not see it")
	}
}

func TestPostgresLastSeenStores(t *testing.T) {
	pool := consumerStatePool(t)
	ctx := context.Background()
	price := repository.NewPostgresListingPriceStore(pool)
	stock := repository.NewPostgresListingStockStore(pool)

	if _, known, err := price.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("unknown price: known=%v err=%v", known, err)
	}
	// Setting only the price leaves the stock "unknown", not zero.
	if err := price.Set(ctx, "L1", 2_000_000); err != nil {
		t.Fatal(err)
	}
	if p, known, err := price.Get(ctx, "L1"); err != nil || !known || p != 2_000_000 {
		t.Fatalf("price = %d known=%v err=%v", p, known, err)
	}
	if _, known, _ := stock.Get(ctx, "L1"); known {
		t.Fatal("stock must stay unknown until set")
	}
	if err := stock.Set(ctx, "L1", 0); err != nil {
		t.Fatal(err)
	}
	if s, known, err := stock.Get(ctx, "L1"); err != nil || !known || s != 0 {
		t.Fatalf("stock = %d known=%v err=%v", s, known, err)
	}
	if err := price.Set(ctx, "L1", 500_000); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := price.Get(ctx, "L1"); p != 500_000 {
		t.Fatalf("price after update = %d", p)
	}
	if s, known, _ := stock.Get(ctx, "L1"); !known || s != 0 {
		t.Fatal("updating price must not touch stock")
	}
}

// End to end against Postgres: a listing seen at 2,000,000, a restart (fresh
// consumer and fresh stores over the same database), then a drop to 500,000 still
// notifies; and a redelivered record afterwards notifies nobody again.
func TestPriceDropAfterRestartAndRedelivery_Postgres(t *testing.T) {
	pool := consumerStatePool(t)
	ctx := context.Background()
	notif := &fakeNotif{}

	if err := pgBundle(pool).listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, priceEnvelope(t, "pg_evt_1", "listing_1", 2_000_000)); err != nil {
		t.Fatal(err)
	}
	drop := priceEnvelope(t, "pg_evt_2", "listing_1", 500_000)
	if err := pgBundle(pool).listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("price drop after restart: %d notifications, want 1", notif.count())
	}
	if err := pgBundle(pool).listingConsumer(notif, priceDropSubs()).HandleRaw(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if notif.count() != 1 {
		t.Fatalf("redelivery after restart: %d notifications, want 1", notif.count())
	}
}
