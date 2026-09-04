package service

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

func newSponsoredSvc() (*SponsoredService, *repository.InMemoryAdCampaignRepository) {
	repo := repository.NewInMemoryAdCampaignRepository()
	return NewSponsoredService(repo, nil), repo
}

// TestCreateAdCampaignHappyPath covers the mock create: a seller's campaign is
// persisted active with its budget/bid, and the seller id is preserved.
func TestCreateAdCampaignHappyPath(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := context.Background()

	c, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{
		SellerID:  "seller-1",
		ListingID: "listing-1",
		Budget:    1000000,
		Bid:       5000,
	})
	if err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	if c.ID == "" {
		t.Fatal("expected generated id")
	}
	if c.SellerID != "seller-1" || c.ListingID != "listing-1" {
		t.Fatalf("unexpected campaign: %+v", c)
	}
	if c.Budget != 1000000 || c.Bid != 5000 {
		t.Fatalf("budget/bid not preserved: %+v", c)
	}
	if c.Status != repository.AdCampaignStatusActive {
		t.Fatalf("status = %d, want active(%d)", c.Status, repository.AdCampaignStatusActive)
	}
}

// TestListSponsoredSlotsBidOrdered verifies active campaigns' listing ids are
// returned best-bid first.
func TestListSponsoredSlotsBidOrdered(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := context.Background()

	// Insert out of bid order; expect the response sorted by bid descending.
	mk := func(seller, listing string, bid int64) {
		if _, err := svc.CreateAdCampaign(ctx, CreateAdCampaignParams{
			SellerID: seller, ListingID: listing, Budget: 10 * bid, Bid: bid,
		}); err != nil {
			t.Fatalf("CreateAdCampaign(%s): %v", listing, err)
		}
	}
	mk("seller-1", "listing-low", 100)
	mk("seller-2", "listing-high", 900)
	mk("seller-3", "listing-mid", 500)

	slots, err := svc.ListSponsoredSlots(ctx, "homepage")
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	want := []string{"listing-high", "listing-mid", "listing-low"}
	if len(slots) != len(want) {
		t.Fatalf("got %d slots, want %d: %v", len(slots), len(want), slots)
	}
	for i, w := range want {
		if slots[i] != w {
			t.Fatalf("slot[%d] = %q, want %q (full: %v)", i, slots[i], w, slots)
		}
	}
}

// TestListSponsoredSlotsEmpty covers the anonymous/empty path: no active campaigns
// yields an empty (non-nil-panicking) slot list, and an empty context is accepted.
func TestListSponsoredSlotsEmpty(t *testing.T) {
	svc, _ := newSponsoredSvc()
	ctx := context.Background()

	slots, err := svc.ListSponsoredSlots(ctx, "")
	if err != nil {
		t.Fatalf("ListSponsoredSlots(empty): %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("want no slots, got %v", slots)
	}
}

// TestCreateAdCampaignValidation covers the invalid-input edges: a missing seller,
// missing listing, or negative budget/bid are rejected and nothing is persisted.
func TestCreateAdCampaignValidation(t *testing.T) {
	svc, repo := newSponsoredSvc()
	ctx := context.Background()

	cases := []struct {
		name string
		p    CreateAdCampaignParams
	}{
		{"missing seller", CreateAdCampaignParams{SellerID: "", ListingID: "l1", Bid: 1}},
		{"missing listing", CreateAdCampaignParams{SellerID: "s1", ListingID: "", Bid: 1}},
		{"negative budget", CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Budget: -1}},
		{"negative bid", CreateAdCampaignParams{SellerID: "s1", ListingID: "l1", Bid: -1}},
	}
	for _, tc := range cases {
		if _, err := svc.CreateAdCampaign(ctx, tc.p); err != ErrInvalidAdCampaign {
			t.Fatalf("%s: err = %v, want ErrInvalidAdCampaign", tc.name, err)
		}
	}

	// No active campaign should have been created by any rejected call.
	slots, err := svc.ListSponsoredSlots(ctx, "")
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("rejected creates leaked campaigns: %v", slots)
	}
	if active, _ := repo.ListActive(ctx, 10); len(active) != 0 {
		t.Fatalf("repo has %d active campaigns after rejects, want 0", len(active))
	}
}
