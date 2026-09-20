package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func TestDuckDBRepository_OrderFactsAndFunnel(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	d1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Write tracking events
	trackingEvents := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "impression", OccurredAt: d1},
		{EventID: "e2", EventType: "impression", OccurredAt: d1},
		{EventID: "e3", EventType: "view", OccurredAt: d1},
		{EventID: "e4", EventType: "add_to_cart", OccurredAt: d1},
		{EventID: "e5", EventType: "view", OccurredAt: d2},
	}
	if err := w.Write(ctx, trackingEvents); err != nil {
		t.Fatalf("Write tracking events: %v", err)
	}

	// Write order facts
	orderFacts := []*warehouse.OrderFactRecord{
		{
			EventID:    "evt-1",
			OrderID:    "o-1",
			ListingID:  "lst-a",
			VariantID:  "sku-a",
			SellerID:   "seller-1",
			Quantity:   2,
			UnitPrice:  500,
			Currency:   "VND",
			OccurredAt: d1,
			Status:     "PAID",
		},
		{
			EventID:    "evt-2",
			OrderID:    "o-2",
			ListingID:  "lst-a",
			VariantID:  "sku-a",
			SellerID:   "seller-1",
			Quantity:   1,
			UnitPrice:  3000,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
		{
			EventID:    "evt-3",
			OrderID:    "o-3",
			ListingID:  "lst-b",
			VariantID:  "sku-b",
			SellerID:   "seller-1",
			Quantity:   5,
			UnitPrice:  100,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
		// seller-2 facts
		{
			EventID:    "evt-4",
			OrderID:    "o-9",
			ListingID:  "lst-z",
			VariantID:  "sku-z",
			SellerID:   "seller-2",
			Quantity:   1,
			UnitPrice:  99999,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
	}
	if err := w.WriteOrderFacts(ctx, orderFacts); err != nil {
		t.Fatalf("WriteOrderFacts: %v", err)
	}

	repo := query.NewDuckDBRepository(w.DB())
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	// Test Funnel
	funnel, err := repo.SellerFunnel(ctx, "seller-1", from, to)
	if err != nil {
		t.Fatalf("SellerFunnel: %v", err)
	}
	if funnel.Impressions != 2 || funnel.Views != 2 || funnel.Adds != 1 || funnel.Orders != 3 {
		t.Errorf("funnel = %+v, want impressions=2 views=2 adds=1 orders=3", funnel)
	}

	// Test Revenue Breakdown
	breakdown, err := repo.RevenueBreakdown(ctx, "seller-1", from, to, 10)
	if err != nil {
		t.Fatalf("RevenueBreakdown: %v", err)
	}
	if len(breakdown.Days) != 2 {
		t.Fatalf("days len = %d, want 2", len(breakdown.Days))
	}
	if breakdown.Days[0].Day != "2026-09-01" || breakdown.Days[0].Revenue != 1000 || breakdown.Days[0].OrderCount != 1 {
		t.Errorf("day[0] = %+v, want 2026-09-01/1000/1", breakdown.Days[0])
	}
	if breakdown.Days[1].Day != "2026-09-02" || breakdown.Days[1].Revenue != 3500 || breakdown.Days[1].OrderCount != 2 {
		t.Errorf("day[1] = %+v, want 2026-09-02/3500/2", breakdown.Days[1])
	}

	if len(breakdown.TopSkus) != 2 {
		t.Fatalf("top_skus len = %d, want 2", len(breakdown.TopSkus))
	}
	if breakdown.TopSkus[0].SKU != "sku-a" || breakdown.TopSkus[0].Revenue != 4000 || breakdown.TopSkus[0].UnitsSold != 3 {
		t.Errorf("top_skus[0] = %+v, want sku-a/4000/3", breakdown.TopSkus[0])
	}
	if breakdown.TopSkus[1].SKU != "sku-b" || breakdown.TopSkus[1].Revenue != 500 || breakdown.TopSkus[1].UnitsSold != 5 {
		t.Errorf("top_skus[1] = %+v, want sku-b/500/5", breakdown.TopSkus[1])
	}

	// Isolation test for seller-2
	b2, err := repo.RevenueBreakdown(ctx, "seller-2", from, to, 10)
	if err != nil {
		t.Fatalf("RevenueBreakdown seller-2: %v", err)
	}
	if len(b2.TopSkus) != 1 || b2.TopSkus[0].SKU != "sku-z" || b2.TopSkus[0].Revenue != 99999 {
		t.Errorf("seller-2 top_skus = %+v, want single sku-z/99999", b2.TopSkus)
	}

	// Test Demand Forecast
	fc, err := repo.DemandForecast(ctx, "seller-1", "lst-a", 14)
	if err != nil {
		t.Fatalf("DemandForecast: %v", err)
	}
	if fc.SellerID != "seller-1" || fc.ListingID != "lst-a" {
		t.Errorf("fc seller/listing = %s/%s, want seller-1/lst-a", fc.SellerID, fc.ListingID)
	}
	if len(fc.DailyForecast) != 14 {
		t.Fatalf("daily forecast len = %d, want 14", len(fc.DailyForecast))
	}
	if fc.DailyForecast[0].P10 > fc.DailyForecast[0].P50 || fc.DailyForecast[0].P50 > fc.DailyForecast[0].P90 {
		t.Errorf("invalid quantile ordering: p10=%v, p50=%v, p90=%v", fc.DailyForecast[0].P10, fc.DailyForecast[0].P50, fc.DailyForecast[0].P90)
	}
}

