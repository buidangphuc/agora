package storefake_test

import (
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func voucherEvent(v store.Voucher) (outbox.Row, error) {
	return outbox.Row{EventID: "ev-" + v.ID, AggregateType: "voucher", AggregateID: v.ID, EventType: "platform.promotion.v1.VoucherChanged", Payload: []byte("p")}, nil
}

func campaignEvent(c store.FlashSaleCampaign) (outbox.Row, error) {
	return outbox.Row{EventID: "ev-" + c.ID, AggregateType: "flash_sale", AggregateID: c.ID, EventType: "platform.promotion.v1.FlashSaleChanged", Payload: []byte("p")}, nil
}

func TestInMemoryVoucherCreateCallsBuilderWithStoredEntity(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	var seen store.Voucher
	calls := 0
	v, err := repo.Create(t.Context(), store.Voucher{Code: "B1"}, func(created store.Voucher) (outbox.Row, error) {
		calls++
		seen = created
		return voucherEvent(created)
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if calls != 1 || seen.ID == "" || seen.ID != v.ID {
		t.Fatalf("builder calls=%d seen id=%q stored id=%q, want one call with the stored entity", calls, seen.ID, v.ID)
	}
}

func TestInMemoryVoucherCreateBuilderErrorStoresNothing(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	boom := errors.New("boom")
	if _, err := repo.Create(t.Context(), store.Voucher{Code: "B2"}, func(store.Voucher) (outbox.Row, error) {
		return outbox.Row{}, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("create err = %v, want the builder error", err)
	}
	if _, err := repo.GetByCode(t.Context(), "B2"); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("voucher persisted despite builder error: %v", err)
	}
}

func TestInMemoryFlashSaleCreateCallsBuilderWithStoredEntity(t *testing.T) {
	repo := storefake.NewInMemoryFlashSaleRepository()
	var seen store.FlashSaleCampaign
	c, err := repo.Create(t.Context(), store.FlashSaleCampaign{ListingID: "lb"}, func(created store.FlashSaleCampaign) (outbox.Row, error) {
		seen = created
		return campaignEvent(created)
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if seen.ID == "" || seen.ID != c.ID {
		t.Fatalf("builder saw id %q, stored id %q", seen.ID, c.ID)
	}
}

func TestInMemoryFlashSaleCreateBuilderErrorStoresNothing(t *testing.T) {
	repo := storefake.NewInMemoryFlashSaleRepository()
	boom := errors.New("boom")
	_, err := repo.Create(t.Context(), store.FlashSaleCampaign{ID: "c-x", ListingID: "lb"}, func(store.FlashSaleCampaign) (outbox.Row, error) {
		return outbox.Row{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("create err = %v, want the builder error", err)
	}
	if _, err := repo.GetByID(t.Context(), "c-x"); !errors.Is(err, store.ErrCampaignNotFound) {
		t.Fatalf("campaign persisted despite builder error: %v", err)
	}
}

func TestInMemoryVoucherRepository(t *testing.T) {
	ctx := t.Context()
	repo := storefake.NewInMemoryVoucherRepository()

	v, err := repo.Create(ctx, store.Voucher{Code: "SAVE10", DiscountType: 1, DiscountValue: 10, Quota: 3}, voucherEvent)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.ID == "" {
		t.Fatal("expected generated id")
	}

	// Duplicate code is rejected.
	if _, err := repo.Create(ctx, store.Voucher{Code: "SAVE10"}, voucherEvent); err == nil {
		t.Fatal("expected duplicate code error")
	}

	got, err := repo.GetByCode(ctx, "SAVE10")
	if err != nil || got.ID != v.ID {
		t.Fatalf("GetByCode: got=%+v err=%v", got, err)
	}
	if _, err := repo.GetByID(ctx, v.ID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := repo.GetByCode(ctx, "NOPE"); !errors.Is(err, store.ErrVoucherNotFound) {
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
	if _, err := repo.Create(ctx, store.Voucher{Code: "SHOP1", SellerID: "s1"}, voucherEvent); err != nil {
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
	ctx := t.Context()
	repo := storefake.NewInMemoryReservationRepository()

	first, created, err := repo.Create(ctx, store.Reservation{
		ReservationID: "resv-1", VoucherID: "v1", BuyerID: "b1", DiscountAmount: 500,
	})
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}

	// Same reservation_id → existing row, created=false, original discount kept.
	second, created, err := repo.Create(ctx, store.Reservation{
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

	if err := repo.UpdateStatus(ctx, "resv-1", store.ReservationCommitted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.GetByReservationID(ctx, "resv-1")
	if got.Status != store.ReservationCommitted {
		t.Fatalf("status=%q, want committed", got.Status)
	}
	if _, err := repo.GetByReservationID(ctx, "missing"); !errors.Is(err, store.ErrReservationNotFound) {
		t.Fatalf("expected ErrReservationNotFound, got %v", err)
	}
}

func TestInMemoryAdCampaignRepository(t *testing.T) {
	ctx := t.Context()
	repo := storefake.NewInMemoryAdCampaignRepository()

	// Two sellers, three active campaigns at different bids.
	a, err := repo.Create(ctx, store.AdCampaign{SellerID: "s1", ListingID: "l1", Bid: 100})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if a.ID == "" || a.Status != store.AdCampaignStatusActive {
		t.Fatalf("expected generated id + active status, got %+v", a)
	}
	if _, err := repo.Create(ctx, store.AdCampaign{SellerID: "s2", ListingID: "l2", Bid: 900}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	// A paused campaign must be excluded from ListActive.
	if _, err := repo.Create(ctx, store.AdCampaign{SellerID: "s1", ListingID: "l3", Bid: 5000, Status: store.AdCampaignStatusPaused}); err != nil {
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
	if _, err := repo.GetByID(ctx, "nope"); !errors.Is(err, store.ErrAdCampaignNotFound) {
		t.Fatalf("expected ErrAdCampaignNotFound, got %v", err)
	}
}

func TestInMemoryFlashSaleRepository(t *testing.T) {
	ctx := t.Context()
	repo := storefake.NewInMemoryFlashSaleRepository()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	live, err := repo.Create(ctx, store.FlashSaleCampaign{
		ListingID: "l1", StockCap: 100, StockSold: 10,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, campaignEvent)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, store.FlashSaleCampaign{
		ListingID: "l1", StockCap: 5,
		StartsAt: now.Add(-3 * time.Hour), EndsAt: now.Add(-2 * time.Hour),
	}, campaignEvent); err != nil {
		t.Fatalf("create expired: %v", err)
	}

	got, err := repo.GetActiveByListing(ctx, "l1", now)
	if err != nil || got.ID != live.ID {
		t.Fatalf("GetActiveByListing = %+v err=%v (want %s)", got, err, live.ID)
	}

	if _, err := repo.GetActiveByListing(ctx, "l1", now.Add(48*time.Hour)); !errors.Is(err, store.ErrCampaignNotFound) {
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
