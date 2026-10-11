package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
)

// stubFlags is a fixed-answer feature-flag evaluator for tests.
type stubFlags struct{ enabled bool }

func (s stubFlags) BooleanEnabled(_ context.Context, _ string, _ bool) bool { return s.enabled }

func newFlashSvc(t *testing.T, flagEnabled bool) (*FlashSaleService, *capturePublisher) {
	t.Helper()
	repo := repository.NewInMemoryFlashSaleRepository()
	svc := NewFlashSaleService(repo, nil, stubFlags{enabled: flagEnabled}, nil, nil)
	pub := &capturePublisher{}
	svc.emitter = producer.NewEmitter(pub, nil)
	return svc, pub
}

func TestCreateCampaignEmitsEvent(t *testing.T) {
	svc, pub := newFlashSvc(t, true)
	c, err := svc.CreateCampaign(context.Background(), CreateCampaignParams{
		ListingID: "list-1",
		SalePrice: 90000,
		StockCap:  100,
		StartsAt:  time.Now().Add(-time.Hour),
		EndsAt:    time.Now().Add(time.Hour),
		IsAdmin:   true,
	})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if c.ID == "" {
		t.Fatal("expected generated campaign id")
	}
	if pub.count() != 1 {
		t.Fatalf("expected 1 promotion.events record, got %d", pub.count())
	}
}

func TestGetFlashSaleStockRemaining(t *testing.T) {
	svc, _ := newFlashSvc(t, true)
	ctx := context.Background()
	repo := svc.campaigns.(*repository.InMemoryFlashSaleRepository)
	c, _ := repo.Create(ctx, repository.FlashSaleCampaign{ListingID: "l", StockCap: 100, StockSold: 30})

	remaining, stockCap, err := svc.GetFlashSaleStock(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if remaining != 70 || stockCap != 100 {
		t.Fatalf("remaining=%d cap=%d, want 70/100", remaining, stockCap)
	}

	// Oversold never goes negative.
	c2, _ := repo.Create(ctx, repository.FlashSaleCampaign{ListingID: "l2", StockCap: 10, StockSold: 25})
	remaining, _, err = svc.GetFlashSaleStock(ctx, c2.ID)
	if err != nil {
		t.Fatalf("GetFlashSaleStock oversold: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining=%d, want 0 (floored)", remaining)
	}
}

func TestGetActiveFlashSaleWindow(t *testing.T) {
	svc, _ := newFlashSvc(t, true)
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now }

	// Active campaign (window brackets now).
	active, _ := svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "live", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	// Expired campaign for a different listing.
	_, _ = svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "dead", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour),
	})

	got, ok, err := svc.GetActiveFlashSale(ctx, "live")
	if err != nil {
		t.Fatalf("GetActiveFlashSale live: %v", err)
	}
	if !ok || got.ID != active.ID {
		t.Fatalf("expected active campaign %s, got ok=%v id=%s", active.ID, ok, got.ID)
	}

	_, ok, err = svc.GetActiveFlashSale(ctx, "dead")
	if err != nil {
		t.Fatalf("GetActiveFlashSale dead: %v", err)
	}
	if ok {
		t.Fatal("expired campaign must not be active")
	}
}

func TestFlashSaleKillSwitchSuppresses(t *testing.T) {
	svc, _ := newFlashSvc(t, false) // flag OFF
	ctx := context.Background()
	now := time.Now()
	svc.nowFn = func() time.Time { return now }
	_, _ = svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "live", SalePrice: 1, StockCap: 5, IsAdmin: true,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})

	_, ok, err := svc.GetActiveFlashSale(ctx, "live")
	if err != nil {
		t.Fatalf("GetActiveFlashSale: %v", err)
	}
	if ok {
		t.Fatal("kill-switch OFF must suppress the active flash sale")
	}
}

// fakeListings is an in-memory team-domain listing client: id -> seller id.
type fakeListings struct {
	owners map[string]string
	err    error
	calls  int
}

func (f *fakeListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	seller, ok := f.owners[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}

func TestCreateCampaignOwnership(t *testing.T) {
	owners := map[string]string{"mine": "seller-1", "theirs": "seller-2"}
	cases := []struct {
		name     string
		listings *fakeListings // nil interface when unset
		unset    bool
		params   CreateCampaignParams
		want     codes.Code
	}{
		{"own listing ok", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.OK},
		{"foreign listing denied", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "theirs", CallerID: "seller-1"}, codes.PermissionDenied},
		{"listing not found", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "ghost", CallerID: "seller-1"}, codes.InvalidArgument},
		{"upstream error", &fakeListings{err: status.Error(codes.Internal, "boom")}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"upstream down", &fakeListings{err: errors.New("dial tcp: refused")}, false, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"admin skips lookup", &fakeListings{owners: owners}, false, CreateCampaignParams{ListingID: "theirs", CallerID: "admin-1", IsAdmin: true}, codes.OK},
		{"unset fails closed for seller", nil, true, CreateCampaignParams{ListingID: "mine", CallerID: "seller-1"}, codes.Unavailable},
		{"unset still allows admin", nil, true, CreateCampaignParams{ListingID: "mine", CallerID: "admin-1", IsAdmin: true}, codes.OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newFlashSvc(t, true)
			if !tc.unset {
				svc.listings = tc.listings
			}
			_, err := svc.CreateCampaign(context.Background(), tc.params)
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", got, tc.want, err)
			}
			if tc.params.IsAdmin && tc.listings != nil && tc.listings.calls != 0 {
				t.Fatalf("admin must not trigger an ownership lookup")
			}
		})
	}
}
