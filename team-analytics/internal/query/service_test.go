package query_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
)

func ts(t time.Time) *timestamppb.Timestamp { return timestamppb.New(t) }

// fixtures: two sellers, three days, so tests can assert scoping + aggregation.
func newRepo() *query.MemoryRepository {
	d1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	return query.NewMemoryRepository(
		// seller-1, day 1: funnel signals + one order (sku-a).
		query.Event{SellerID: "seller-1", EventType: "impression", OccurredAt: d1},
		query.Event{SellerID: "seller-1", EventType: "impression", OccurredAt: d1},
		query.Event{SellerID: "seller-1", EventType: "view", OccurredAt: d1},
		query.Event{SellerID: "seller-1", EventType: "add_to_cart", OccurredAt: d1},
		query.Event{SellerID: "seller-1", OrderID: "o-1", Revenue: 1000, SKU: "sku-a", ListingID: "lst-a", Units: 2, OccurredAt: d1},
		// seller-1, day 2: two orders, one repeats sku-a (higher revenue), one sku-b.
		query.Event{SellerID: "seller-1", EventType: "view", OccurredAt: d2},
		query.Event{SellerID: "seller-1", OrderID: "o-2", Revenue: 3000, SKU: "sku-a", ListingID: "lst-a", Units: 1, OccurredAt: d2},
		query.Event{SellerID: "seller-1", OrderID: "o-3", Revenue: 500, SKU: "sku-b", ListingID: "lst-b", Units: 5, OccurredAt: d2},
		// seller-2 noise on the same days — must never leak into seller-1 results.
		query.Event{SellerID: "seller-2", EventType: "impression", OccurredAt: d1},
		query.Event{SellerID: "seller-2", OrderID: "o-9", Revenue: 99999, SKU: "sku-z", ListingID: "lst-z", Units: 9, OccurredAt: d2},
	)
}

// Happy-path funnel: impressions/views/adds by event type + distinct orders.
func TestGetSellerFunnel_HappyPath(t *testing.T) {
	svc := query.NewService(newRepo())
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	resp, err := svc.GetSellerFunnel(context.Background(), &analyticsv1.GetSellerFunnelRequest{
		SellerId: "seller-1", From: ts(from), To: ts(to),
	})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if got := resp.GetImpressions(); got != 2 {
		t.Errorf("impressions = %d, want 2", got)
	}
	if got := resp.GetViews(); got != 2 {
		t.Errorf("views = %d, want 2", got)
	}
	if got := resp.GetAdds(); got != 1 {
		t.Errorf("adds = %d, want 1", got)
	}
	if got := resp.GetOrders(); got != 3 {
		t.Errorf("orders = %d, want 3 (o-1,o-2,o-3)", got)
	}
}

// Happy-path revenue breakdown: per-day totals + top SKUs ordered by revenue.
func TestGetRevenueBreakdown_HappyPath(t *testing.T) {
	svc := query.NewService(newRepo())
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	resp, err := svc.GetRevenueBreakdown(context.Background(), &analyticsv1.GetRevenueBreakdownRequest{
		SellerId: "seller-1", From: ts(from), To: ts(to),
	})
	if err != nil {
		t.Fatalf("GetRevenueBreakdown: %v", err)
	}

	// Days: 2026-09-01 => 1000/1 order, 2026-09-02 => 3500/2 orders, sorted asc.
	if len(resp.GetDays()) != 2 {
		t.Fatalf("days = %d, want 2", len(resp.GetDays()))
	}
	if d := resp.GetDays()[0]; d.GetDay() != "2026-09-01" || d.GetRevenue() != 1000 || d.GetOrderCount() != 1 {
		t.Errorf("day[0] = %+v, want 2026-09-01/1000/1", d)
	}
	if d := resp.GetDays()[1]; d.GetDay() != "2026-09-02" || d.GetRevenue() != 3500 || d.GetOrderCount() != 2 {
		t.Errorf("day[1] = %+v, want 2026-09-02/3500/2", d)
	}

	// Top SKUs: sku-a (1000+3000=4000, 3 units) ranks above sku-b (500, 5 units).
	if len(resp.GetTopSkus()) != 2 {
		t.Fatalf("top_skus = %d, want 2", len(resp.GetTopSkus()))
	}
	if s := resp.GetTopSkus()[0]; s.GetSku() != "sku-a" || s.GetRevenue() != 4000 || s.GetUnitsSold() != 3 || s.GetListingId() != "lst-a" {
		t.Errorf("top_skus[0] = %+v, want sku-a/4000/3/lst-a", s)
	}
	if s := resp.GetTopSkus()[1]; s.GetSku() != "sku-b" || s.GetRevenue() != 500 || s.GetUnitsSold() != 5 {
		t.Errorf("top_skus[1] = %+v, want sku-b/500/5", s)
	}
}

// Empty range: a window with no events yields zeroed funnel + empty breakdown,
// no panics. Uses a window that predates all fixtures.
func TestEmptyRange(t *testing.T) {
	svc := query.NewService(newRepo())
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)

	f, err := svc.GetSellerFunnel(context.Background(), &analyticsv1.GetSellerFunnelRequest{
		SellerId: "seller-1", From: ts(from), To: ts(to),
	})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if f.GetImpressions() != 0 || f.GetViews() != 0 || f.GetAdds() != 0 || f.GetOrders() != 0 {
		t.Errorf("funnel = %+v, want all zero", f)
	}

	b, err := svc.GetRevenueBreakdown(context.Background(), &analyticsv1.GetRevenueBreakdownRequest{
		SellerId: "seller-1", From: ts(from), To: ts(to),
	})
	if err != nil {
		t.Fatalf("GetRevenueBreakdown: %v", err)
	}
	if len(b.GetDays()) != 0 || len(b.GetTopSkus()) != 0 {
		t.Errorf("breakdown = %+v, want empty days and top_skus", b)
	}
}

// Cross-seller isolation: seller-2's high-revenue order/impression must not
// appear in seller-1's results.
func TestSellerIsolation(t *testing.T) {
	svc := query.NewService(newRepo())

	f, err := svc.GetSellerFunnel(context.Background(), &analyticsv1.GetSellerFunnelRequest{SellerId: "seller-2"})
	if err != nil {
		t.Fatalf("GetSellerFunnel: %v", err)
	}
	if f.GetImpressions() != 1 || f.GetOrders() != 1 {
		t.Errorf("seller-2 funnel = %+v, want impressions=1 orders=1", f)
	}

	b, err := svc.GetRevenueBreakdown(context.Background(), &analyticsv1.GetRevenueBreakdownRequest{SellerId: "seller-2"})
	if err != nil {
		t.Fatalf("GetRevenueBreakdown: %v", err)
	}
	for _, s := range b.GetTopSkus() {
		if s.GetSku() == "sku-a" || s.GetSku() == "sku-b" {
			t.Errorf("seller-2 breakdown leaked seller-1 sku %q", s.GetSku())
		}
	}
	// seller-2 has exactly one SKU worth 99999.
	if len(b.GetTopSkus()) != 1 || b.GetTopSkus()[0].GetRevenue() != 99999 {
		t.Errorf("seller-2 top_skus = %+v, want single sku-z/99999", b.GetTopSkus())
	}
}

// Missing seller_id is rejected with InvalidArgument (anonymous/empty input
// handled without a panic or a wrong-owner leak).
func TestMissingSellerID(t *testing.T) {
	svc := query.NewService(newRepo())

	if _, err := svc.GetSellerFunnel(context.Background(), &analyticsv1.GetSellerFunnelRequest{SellerId: "  "}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetSellerFunnel empty seller: err = %v, want InvalidArgument", err)
	}
	if _, err := svc.GetRevenueBreakdown(context.Background(), &analyticsv1.GetRevenueBreakdownRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetRevenueBreakdown missing seller: err = %v, want InvalidArgument", err)
	}
}

// A nil repository must be reported as Unavailable, not panic.
func TestNilRepository(t *testing.T) {
	svc := query.NewService(nil)
	if _, err := svc.GetSellerFunnel(context.Background(), &analyticsv1.GetSellerFunnelRequest{SellerId: "seller-1"}); status.Code(err) != codes.Unavailable {
		t.Errorf("nil repo funnel: err = %v, want Unavailable", err)
	}
}
