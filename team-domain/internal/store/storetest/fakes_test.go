package storetest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

func TestInMemoryStorefrontRoundTripAndBatch(t *testing.T) {
	r := storetest.NewInMemoryStorefrontRepository()
	ctx := t.Context()
	if _, err := r.Upsert(ctx, store.Storefront{SellerID: "a", Slug: "a", DisplayName: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetBySeller(ctx, "a")
	if got.DisplayName != "Alpha" {
		t.Errorf("name lost: %+v", got)
	}
	many, _ := r.GetBySellers(ctx, []string{"a", "zzz"})
	if len(many) != 1 || many[0].DisplayName != "Alpha" {
		t.Errorf("batch mismatch: %+v", many)
	}
}

func enqueue(id string) store.EnqueueFn {
	return func(l store.Listing) (store.OutboxRow, error) {
		return store.OutboxRow{
			EventID: "evt-" + id, AggregateType: "Listing", AggregateID: l.ID,
			EventType: "platform.listing.v1.ListingChanged", Payload: []byte{0x01},
		}, nil
	}
}

func TestInMemoryTxWriterCommitsListingAndEventTogether(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository()
	w := storetest.NewInMemoryTxWriter(repo)
	ctx := t.Context()

	if _, err := w.CreateTx(ctx, store.Listing{ID: "L1", Title: "t", Currency: "VND", Status: "published"}, enqueue("1")); err != nil {
		t.Fatalf("CreateTx: %v", err)
	}
	if _, err := repo.Get(ctx, "L1"); err != nil {
		t.Fatalf("listing not readable through the repo: %v", err)
	}
	rows := w.Rows()
	if len(rows) != 1 || rows[0].EventID != "evt-1" || rows[0].AggregateID != "L1" {
		t.Fatalf("rows = %+v, want the one event of L1", rows)
	}

	w.FailWrite = true
	if _, err := w.CreateTx(ctx, store.Listing{ID: "L2", Title: "t", Currency: "VND", Status: "published"}, enqueue("2")); err == nil {
		t.Fatal("CreateTx with FailWrite must fail")
	}
	if _, err := repo.Get(ctx, "L2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed write left a listing behind: err = %v", err)
	}
	if got := len(w.Rows()); got != 1 {
		t.Fatalf("failed write left an event behind: rows = %d, want 1", got)
	}
}

func TestInMemoryTxWriterHoldAndRestoreSellerListings(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository(
		store.Listing{ID: "A1", Title: "a", Currency: "VND", Status: "published", SellerID: "S1"},
		store.Listing{ID: "B1", Title: "b", Currency: "VND", Status: "published", SellerID: "S2"},
	)
	w := storetest.NewInMemoryTxWriter(repo)
	ctx := t.Context()

	n, err := w.HoldSellerListings(ctx, "S1", "evt-hold", time.Now(), enqueue("hold"))
	if err != nil || n != 1 {
		t.Fatalf("HoldSellerListings n=%d err=%v, want 1/nil", n, err)
	}
	if got, _ := repo.Get(ctx, "A1"); got.Status != "held" {
		t.Fatalf("A1 status = %q, want held", got.Status)
	}
	if got, _ := repo.Get(ctx, "B1"); got.Status != "published" {
		t.Fatalf("B1 status = %q, want untouched", got.Status)
	}
	n, err = w.RestoreSellerListings(ctx, "S1", "evt-restore", time.Now(), enqueue("restore"))
	if err != nil || n != 1 {
		t.Fatalf("RestoreSellerListings n=%d err=%v, want 1/nil", n, err)
	}
	if got, _ := repo.Get(ctx, "A1"); got.Status != "published" {
		t.Fatalf("A1 status = %q, want published after restore", got.Status)
	}
}
