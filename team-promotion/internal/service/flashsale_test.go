package service

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
)

// stubFlags is a fixed-answer feature-flag evaluator for tests.
type stubFlags struct{ enabled bool }

func (s stubFlags) BooleanEnabled(_ context.Context, _ string, _ bool) bool { return s.enabled }

func newFlashSvc(t *testing.T, flagEnabled bool) (*FlashSaleService, *capturePublisher) {
	t.Helper()
	repo := repository.NewInMemoryFlashSaleRepository()
	svc := NewFlashSaleService(repo, nil, stubFlags{enabled: flagEnabled}, nil)
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
		ListingID: "live", SalePrice: 1, StockCap: 5,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	// Expired campaign for a different listing.
	_, _ = svc.CreateCampaign(ctx, CreateCampaignParams{
		ListingID: "dead", SalePrice: 1, StockCap: 5,
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
		ListingID: "live", SalePrice: 1, StockCap: 5,
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
