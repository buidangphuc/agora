package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// Guard for requirement "Revenue sourced only from order facts":
// tracking purchase `value` must never leak into any revenue query.

const (
	guardTrackingValue = 999999
	guardOrderTotal    = 500000
	guardOrphanValue   = 777777
)

func guardMemory(t *testing.T) query.Repository {
	t.Helper()
	d := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	return query.NewMemoryRepository(
		// order line: the only revenue source.
		query.Event{SellerID: "seller-1", OrderID: "o-1", Revenue: guardOrderTotal, SKU: "sku-a", ListingID: "lst-a", Units: 1, OccurredAt: d},
		// tracking purchases: Event has no value field, so they carry zero revenue
		// by construction; one matches o-1, one has no order at all.
		query.Event{SellerID: "seller-1", EventType: "purchase", TransactionID: "o-1", ListingID: "lst-a", OccurredAt: d},
		query.Event{SellerID: "seller-1", EventType: "purchase", TransactionID: "tx-orphan", ListingID: "lst-a", OccurredAt: d},
	)
}

func guardDuck(t *testing.T) query.Repository {
	t.Helper()
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	d := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "p1", EventType: "purchase", OccurredAt: d, ListingID: "lst-a", Value: guardTrackingValue, Currency: "VND", TransactionID: "o-1"},
		{EventID: "p2", EventType: "purchase", OccurredAt: d, ListingID: "lst-a", Value: guardOrphanValue, Currency: "VND", TransactionID: "tx-orphan"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "lst-a", SellerID: "seller-1", UpdatedAt: d},
	}); err != nil {
		t.Fatalf("UpsertListingSellers: %v", err)
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{{
		EventID: "evt-1", OrderID: "o-1", ListingID: "lst-a", VariantID: "sku-a", SellerID: "seller-1",
		Quantity: 1, UnitPrice: guardOrderTotal, Currency: "VND", OccurredAt: d, Status: "PAID",
	}}); err != nil {
		t.Fatalf("WriteOrderFacts: %v", err)
	}
	return query.NewDuckDBRepository(w.DB())
}

func TestRevenueGuard(t *testing.T) {
	repos := map[string]func(*testing.T) query.Repository{"memory": guardMemory, "duckdb": guardDuck}
	ctx := context.Background()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	for name, mk := range repos {
		t.Run(name, func(t *testing.T) {
			repo := mk(t)

			// Scenario: Tracking value excluded from revenue
			t.Run("Tracking value excluded from revenue/breakdown", func(t *testing.T) {
				b, err := repo.RevenueBreakdown(ctx, "seller-1", from, to, 10)
				if err != nil {
					t.Fatalf("RevenueBreakdown: %v", err)
				}
				if len(b.Days) != 1 || b.Days[0].Revenue != guardOrderTotal || b.Days[0].OrderCount != 1 {
					t.Errorf("days = %+v, want one day 500000/1 order", b.Days)
				}
				if len(b.TopSkus) != 1 || b.TopSkus[0].Revenue != guardOrderTotal {
					t.Errorf("top_skus = %+v, want one sku with 500000", b.TopSkus)
				}
			})
			t.Run("Tracking value excluded from revenue/platform summary", func(t *testing.T) {
				s, err := repo.PlatformOrderSummary(ctx, since)
				if err != nil {
					t.Fatalf("PlatformOrderSummary: %v", err)
				}
				if s.GMV != guardOrderTotal || s.OrderCount != 1 {
					t.Errorf("summary = %+v, want GMV 500000 / 1 order", s)
				}
			})
			t.Run("Tracking value excluded from revenue/recent orders", func(t *testing.T) {
				o, err := repo.RecentOrders(ctx, 10)
				if err != nil {
					t.Fatalf("RecentOrders: %v", err)
				}
				if len(o) != 1 || o[0].OrderID != "o-1" || o[0].Total != guardOrderTotal {
					t.Errorf("orders = %+v, want only o-1 total 500000", o)
				}
			})

			// Scenario: Tracking purchase without order not counted as revenue
			t.Run("Tracking purchase without order not counted as revenue", func(t *testing.T) {
				b, err := repo.RevenueBreakdown(ctx, "seller-1", from, to, 10)
				if err != nil {
					t.Fatalf("RevenueBreakdown: %v", err)
				}
				var total int64
				for _, d := range b.Days {
					total += d.Revenue
				}
				if total != guardOrderTotal {
					t.Errorf("total revenue = %d, want 500000 (orphan tx-orphan contributes 0)", total)
				}
				s, err := repo.PlatformOrderSummary(ctx, since)
				if err != nil {
					t.Fatalf("PlatformOrderSummary: %v", err)
				}
				if s.GMV != guardOrderTotal {
					t.Errorf("GMV = %d, want 500000", s.GMV)
				}
				o, err := repo.RecentOrders(ctx, 10)
				if err != nil {
					t.Fatalf("RecentOrders: %v", err)
				}
				for _, r := range o {
					if r.OrderID == "tx-orphan" {
						t.Errorf("orphan purchase surfaced as order: %+v", r)
					}
				}
			})
		})
	}
}
