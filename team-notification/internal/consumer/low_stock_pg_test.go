package consumer

import (
	"context"
	"testing"

	listingv1 "github.com/buidangphuc/team-notification/generated/platform/listing/v1"
	"github.com/buidangphuc/team-notification/internal/repository"
)

func lowStockPool(t *testing.T) (ctx context.Context, store *repository.PostgresLowStockStore, restart func() *ListingConsumer, notif *fakeNotif) {
	t.Helper()
	pool := consumerStatePool(t)
	ctx = context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM listing_low_stock_state`); err != nil {
		t.Fatalf("reset listing_low_stock_state: %v", err)
	}
	notif = &fakeNotif{}
	store = repository.NewPostgresLowStockStore(pool)
	restart = func() *ListingConsumer {
		return NewListingConsumer(notif, &fakeSubs{}, nil,
			WithDeduper(repository.NewPostgresProcessedEventRepo(pool)),
			WithPriceStateStore(repository.NewPostgresListingPriceStore(pool)),
			WithStockStateStore(repository.NewPostgresListingStockStore(pool)),
			WithLowStockStore(repository.NewPostgresLowStockStore(pool)),
		)
	}
	return
}

// Scenario: Order work: Low-stock state survives a restart
func TestLowStockStateDurableAcrossRestart(t *testing.T) {
	ctx, _, restart, notif := lowStockPool(t)

	first := restart()
	if err := first.HandleRaw(ctx, snapshotEnv(t, "pg_low_1", "L1", "S1", lowStockListingTtl, 8, listingv1.ChangeType_CHANGE_TYPE_UPDATED)); err != nil {
		t.Fatal(err)
	}
	if err := first.HandleRaw(ctx, stockChangedEnv(t, "pg_low_2", "L1", 5)); err != nil {
		t.Fatal(err)
	}
	if got := len(lowNotifs(notif, "S1")); got != 1 {
		t.Fatalf("crossing: want 1, got %d", got)
	}

	second := restart() // fresh consumer and stores over the same database
	if err := second.HandleRaw(ctx, stockChangedEnv(t, "pg_low_3", "L1", 4)); err != nil {
		t.Fatal(err)
	}
	if got := len(lowNotifs(notif, "S1")); got != 1 {
		t.Fatalf("after restart: want still 1, got %d", got)
	}
}

// Scenario: Order work: A deleted listing drops its low-stock state
func TestPostgresLowStockStoreDelete(t *testing.T) {
	ctx, store, _, _ := lowStockPool(t)

	if _, _, _, known, err := store.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("fresh: known=%v err=%v", known, err)
	}
	if err := store.Upsert(ctx, "L1", "S1", "Giày", false); err != nil {
		t.Fatal(err)
	}
	seller, title, low, known, err := store.Get(ctx, "L1")
	if err != nil || !known || seller != "S1" || title != "Giày" || low {
		t.Fatalf("after upsert: %q %q low=%v known=%v err=%v", seller, title, low, known, err)
	}
	if err := store.SetLow(ctx, "L1", true); err != nil {
		t.Fatal(err)
	}
	if _, _, low, _, _ := store.Get(ctx, "L1"); !low {
		t.Fatal("SetLow(true) not persisted")
	}
	if err := store.Delete(ctx, "L1"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, known, err := store.Get(ctx, "L1"); err != nil || known {
		t.Fatalf("after delete: known=%v err=%v", known, err)
	}
	if err := store.Delete(ctx, "L1"); err != nil {
		t.Fatalf("deleting an absent row must not error: %v", err)
	}
}
