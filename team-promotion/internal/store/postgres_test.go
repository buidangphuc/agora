package store_test

import (
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func TestPostgresRepositories(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	now := time.Now().UTC()

	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	rrepo := store.NewPostgresReservationRepository(pool)
	frepo := store.NewPostgresFlashSaleRepository(pool, newOutbox(t, pool))

	// Voucher round-trip + used increment.
	v, err := vrepo.Create(ctx, store.Voucher{
		Code: "PG10", Scope: 2, DiscountType: 1, DiscountValue: 10, Quota: 5,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, voucherEvent)
	if err != nil {
		t.Fatalf("voucher create: %v", err)
	}
	if got, err := vrepo.GetByCode(ctx, "PG10"); err != nil || got.ID != v.ID {
		t.Fatalf("GetByCode: got=%+v err=%v", got, err)
	}
	if err := vrepo.IncrementUsed(ctx, v.ID, 1); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}

	// Reservation idempotency via UNIQUE(reservation_id).
	_, created, err := rrepo.Create(ctx, store.Reservation{
		ReservationID: "pg-resv", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 500,
	})
	if err != nil || !created {
		t.Fatalf("first reservation: created=%v err=%v", created, err)
	}
	_, created, err = rrepo.Create(ctx, store.Reservation{
		ReservationID: "pg-resv", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 999,
	})
	if err != nil {
		t.Fatalf("second reservation: %v", err)
	}
	if created {
		t.Fatal("second reservation must be idempotent (created=false)")
	}
	got, err := rrepo.GetByReservationID(ctx, "pg-resv")
	if err != nil || got.DiscountAmount != 500 {
		t.Fatalf("reservation kept wrong discount: %+v err=%v", got, err)
	}

	// Flash-sale active window.
	c, err := frepo.Create(ctx, store.FlashSaleCampaign{
		ListingID: "pg-listing", SalePrice: 100, StockCap: 50, StockSold: 5,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, campaignEvent)
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	activeC, err := frepo.GetActiveByListing(ctx, "pg-listing", now)
	if err != nil || activeC.ID != c.ID {
		t.Fatalf("GetActiveByListing: got=%+v err=%v", activeC, err)
	}
}
