package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

func TestInMemoryVoucherRepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryVoucherRepository()

	v, err := repo.Create(ctx, repository.Voucher{Code: "SAVE10", DiscountType: 1, DiscountValue: 10, Quota: 3})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.ID == "" {
		t.Fatal("expected generated id")
	}

	// Duplicate code is rejected.
	if _, err := repo.Create(ctx, repository.Voucher{Code: "SAVE10"}); err == nil {
		t.Fatal("expected duplicate code error")
	}

	got, err := repo.GetByCode(ctx, "SAVE10")
	if err != nil || got.ID != v.ID {
		t.Fatalf("GetByCode: got=%+v err=%v", got, err)
	}
	if _, err := repo.GetByID(ctx, v.ID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := repo.GetByCode(ctx, "NOPE"); err != repository.ErrVoucherNotFound {
		t.Fatalf("expected ErrVoucherNotFound, got %v", err)
	}

	if err := repo.IncrementUsed(ctx, v.ID, 2); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}
	got, _ = repo.GetByID(ctx, v.ID)
	if got.Used != 2 {
		t.Fatalf("used=%d, want 2", got.Used)
	}

	// List by seller.
	if _, err := repo.Create(ctx, repository.Voucher{Code: "SHOP1", SellerID: "s1"}); err != nil {
		t.Fatalf("create shop voucher: %v", err)
	}
	list, err := repo.List(ctx, "s1", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Code != "SHOP1" {
		t.Fatalf("List(s1) = %+v", list)
	}
}

func TestInMemoryReservationIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReservationRepository()

	first, created, err := repo.Create(ctx, repository.Reservation{
		ReservationID: "resv-1", VoucherID: "v1", BuyerID: "b1", DiscountAmount: 500,
	})
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}

	// Same reservation_id → existing row, created=false, original discount kept.
	second, created, err := repo.Create(ctx, repository.Reservation{
		ReservationID: "resv-1", VoucherID: "v1", BuyerID: "b1", DiscountAmount: 999,
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if created {
		t.Fatal("second create must not report created=true")
	}
	if second.ID != first.ID || second.DiscountAmount != 500 {
		t.Fatalf("idempotency broken: first=%+v second=%+v", first, second)
	}

	if err := repo.UpdateStatus(ctx, "resv-1", repository.ReservationCommitted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.GetByReservationID(ctx, "resv-1")
	if got.Status != repository.ReservationCommitted {
		t.Fatalf("status=%q, want committed", got.Status)
	}
	if _, err := repo.GetByReservationID(ctx, "missing"); err != repository.ErrReservationNotFound {
		t.Fatalf("expected ErrReservationNotFound, got %v", err)
	}
}

func TestInMemoryAdCampaignRepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryAdCampaignRepository()

	// Two sellers, three active campaigns at different bids.
	a, err := repo.Create(ctx, repository.AdCampaign{SellerID: "s1", ListingID: "l1", Bid: 100})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if a.ID == "" || a.Status != repository.AdCampaignStatusActive {
		t.Fatalf("expected generated id + active status, got %+v", a)
	}
	if _, err := repo.Create(ctx, repository.AdCampaign{SellerID: "s2", ListingID: "l2", Bid: 900}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	// A paused campaign must be excluded from ListActive.
	if _, err := repo.Create(ctx, repository.AdCampaign{SellerID: "s1", ListingID: "l3", Bid: 5000, Status: repository.AdCampaignStatusPaused}); err != nil {
		t.Fatalf("create paused: %v", err)
	}

	// ListActive is bid-ordered and excludes the paused campaign.
	active, err := repo.ListActive(ctx, 10)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("ListActive len = %d, want 2 (paused excluded): %+v", len(active), active)
	}
	if active[0].ListingID != "l2" || active[1].ListingID != "l1" {
		t.Fatalf("ListActive not bid-ordered: %+v", active)
	}

	// Cross-user isolation: ListBySeller returns only that seller's campaigns.
	s1, err := repo.ListBySeller(ctx, "s1", 10, 0)
	if err != nil {
		t.Fatalf("ListBySeller(s1): %v", err)
	}
	if len(s1) != 2 {
		t.Fatalf("ListBySeller(s1) len = %d, want 2", len(s1))
	}
	for _, c := range s1 {
		if c.SellerID != "s1" {
			t.Fatalf("ListBySeller(s1) leaked seller %q", c.SellerID)
		}
	}

	// GetByID hits and misses.
	if got, err := repo.GetByID(ctx, a.ID); err != nil || got.ID != a.ID {
		t.Fatalf("GetByID: got=%+v err=%v", got, err)
	}
	if _, err := repo.GetByID(ctx, "nope"); err != repository.ErrAdCampaignNotFound {
		t.Fatalf("expected ErrAdCampaignNotFound, got %v", err)
	}
}

func TestInMemoryFlashSaleRepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryFlashSaleRepository()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	live, err := repo.Create(ctx, repository.FlashSaleCampaign{
		ListingID: "l1", StockCap: 100, StockSold: 10,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, repository.FlashSaleCampaign{
		ListingID: "l1", StockCap: 5,
		StartsAt: now.Add(-3 * time.Hour), EndsAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create expired: %v", err)
	}

	got, err := repo.GetActiveByListing(ctx, "l1", now)
	if err != nil || got.ID != live.ID {
		t.Fatalf("GetActiveByListing = %+v err=%v (want %s)", got, err, live.ID)
	}

	if _, err := repo.GetActiveByListing(ctx, "l1", now.Add(48*time.Hour)); err != repository.ErrCampaignNotFound {
		t.Fatalf("expected no active campaign in the far future, got %v", err)
	}

	activeList, err := repo.ListActive(ctx, now, 10, 0)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(activeList) != 1 || activeList[0].ID != live.ID {
		t.Fatalf("ListActive = %+v", activeList)
	}
}
