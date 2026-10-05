package query_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/interceptor"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// principalCtx runs the real unary interceptor so the test exercises the same
// metadata → Principal → RequireScopes path the gateway-forwarded call takes.
func principalCtx(t *testing.T, scopes string) context.Context {
	t.Helper()
	in := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-principal-id", "u-1", "x-principal-type", "user", "x-principal-scopes", scopes))
	var out context.Context
	_, err := interceptor.UnaryServerInterceptor()(in, nil, nil, func(c context.Context, _ any) (any, error) {
		out = c
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	return out
}

func adminCtx(t *testing.T) context.Context { return principalCtx(t, "admin,listing.read") }

func ordersRepo(now time.Time) *query.MemoryRepository {
	return query.NewMemoryRepository(
		// o-1: two lines, 2*150000 + 1*50000 = 350000, paid 1h ago.
		query.Event{SellerID: "s-1", OrderID: "o-1", Revenue: 300000, OccurredAt: now.Add(-time.Hour)},
		query.Event{SellerID: "s-1", OrderID: "o-1", Revenue: 50000, OccurredAt: now.Add(-time.Hour)},
		// o-2: newer, another seller.
		query.Event{SellerID: "s-2", OrderID: "o-2", Revenue: 1000, OccurredAt: now.Add(-time.Minute)},
		// o-3: just inside the 24h edge.
		query.Event{SellerID: "s-1", OrderID: "o-3", Revenue: 200, OccurredAt: now.Add(-23*time.Hour - 59*time.Minute)},
		// o-4: just outside the 24h edge.
		query.Event{SellerID: "s-1", OrderID: "o-4", Revenue: 9999, OccurredAt: now.Add(-24*time.Hour - time.Minute)},
		// behavioural row, not an order.
		query.Event{SellerID: "s-1", EventType: "view", OccurredAt: now},
	)
}

func TestGetPlatformOrderSummary_CountsGMVAndWindowEdge(t *testing.T) {
	svc := query.NewService(ordersRepo(time.Now()))
	resp, err := svc.GetPlatformOrderSummary(adminCtx(t), &analyticsv1.GetPlatformOrderSummaryRequest{})
	if err != nil {
		t.Fatalf("GetPlatformOrderSummary: %v", err)
	}
	if resp.GetOrderCount() != 3 {
		t.Errorf("order_count = %d, want 3 (o-1,o-2,o-3; o-4 is outside 24h)", resp.GetOrderCount())
	}
	if resp.GetGmv() != 351200 {
		t.Errorf("gmv = %d, want 351200", resp.GetGmv())
	}
}

func TestGetPlatformOrderSummary_ExplicitWindow(t *testing.T) {
	svc := query.NewService(ordersRepo(time.Now()))
	resp, err := svc.GetPlatformOrderSummary(adminCtx(t), &analyticsv1.GetPlatformOrderSummaryRequest{Window: durationpb.New(2 * time.Hour)})
	if err != nil {
		t.Fatalf("GetPlatformOrderSummary: %v", err)
	}
	if resp.GetOrderCount() != 2 || resp.GetGmv() != 351000 {
		t.Errorf("2h window = %d orders / %d gmv, want 2 / 351000", resp.GetOrderCount(), resp.GetGmv())
	}
}

func TestGetPlatformOrderSummary_EmptyTable(t *testing.T) {
	svc := query.NewService(query.NewMemoryRepository())
	resp, err := svc.GetPlatformOrderSummary(adminCtx(t), &analyticsv1.GetPlatformOrderSummaryRequest{})
	if err != nil {
		t.Fatalf("GetPlatformOrderSummary: %v", err)
	}
	if resp.GetOrderCount() != 0 || resp.GetGmv() != 0 {
		t.Errorf("empty = %d / %d, want 0 / 0", resp.GetOrderCount(), resp.GetGmv())
	}
}

func TestListRecentOrders_NewestFirstAndLimit(t *testing.T) {
	svc := query.NewService(ordersRepo(time.Now()))
	resp, err := svc.ListRecentOrders(adminCtx(t), &analyticsv1.ListRecentOrdersRequest{Limit: 2})
	if err != nil {
		t.Fatalf("ListRecentOrders: %v", err)
	}
	got := resp.GetOrders()
	if len(got) != 2 {
		t.Fatalf("orders len = %d, want 2", len(got))
	}
	if got[0].GetOrderId() != "o-2" || got[0].GetSellerId() != "s-2" || got[0].GetTotal() != 1000 {
		t.Errorf("orders[0] = %v, want o-2/s-2/1000", got[0])
	}
	if got[1].GetOrderId() != "o-1" || got[1].GetTotal() != 350000 {
		t.Errorf("orders[1] = %v, want o-1 with total 350000 (lines summed)", got[1])
	}
	if got[0].GetPaidAt() == nil {
		t.Error("paid_at must be set")
	}
}

func TestListRecentOrders_EmptyTable(t *testing.T) {
	svc := query.NewService(query.NewMemoryRepository())
	resp, err := svc.ListRecentOrders(adminCtx(t), &analyticsv1.ListRecentOrdersRequest{})
	if err != nil {
		t.Fatalf("ListRecentOrders: %v", err)
	}
	if len(resp.GetOrders()) != 0 {
		t.Errorf("orders = %v, want empty", resp.GetOrders())
	}
}

func TestPlatformOrderRPCs_NonAdminDenied(t *testing.T) {
	svc := query.NewService(ordersRepo(time.Now()))
	cases := map[string]context.Context{
		"buyer":     principalCtx(t, "listing.read,order.read"),
		"anonymous": context.Background(),
	}
	want := map[string]codes.Code{"buyer": codes.PermissionDenied, "anonymous": codes.Unauthenticated}
	for name, ctx := range cases {
		_, err := svc.GetPlatformOrderSummary(ctx, &analyticsv1.GetPlatformOrderSummaryRequest{})
		if status.Code(err) != want[name] {
			t.Errorf("%s GetPlatformOrderSummary code = %v, want %v", name, status.Code(err), want[name])
		}
		_, err = svc.ListRecentOrders(ctx, &analyticsv1.ListRecentOrdersRequest{})
		if status.Code(err) != want[name] {
			t.Errorf("%s ListRecentOrders code = %v, want %v", name, status.Code(err), want[name])
		}
	}
}

// The DuckDB adapter must agree with the in-memory one on real order_facts rows.
func TestDuckDBRepository_PlatformOrders(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	now := time.Now().UTC()
	fact := func(id, order, seller string, qty int32, price int64, at time.Time) *warehouse.OrderFactRecord {
		return &warehouse.OrderFactRecord{EventID: id, OrderID: order, ListingID: "l", SellerID: seller,
			Quantity: qty, UnitPrice: price, Currency: "VND", OccurredAt: at, Status: "PAID"}
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		fact("e1", "o-1", "s-1", 2, 150000, now.Add(-time.Hour)),
		fact("e2", "o-1", "s-1", 1, 50000, now.Add(-time.Hour)),
		fact("e3", "o-2", "s-2", 1, 1000, now.Add(-time.Minute)),
		fact("e4", "o-old", "s-1", 1, 9999, now.Add(-25*time.Hour)),
	}); err != nil {
		t.Fatalf("WriteOrderFacts: %v", err)
	}
	repo := query.NewDuckDBRepository(w.DB())

	sum, err := repo.PlatformOrderSummary(ctx, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PlatformOrderSummary: %v", err)
	}
	if sum.OrderCount != 2 || sum.GMV != 351000 {
		t.Errorf("summary = %+v, want 2 orders / 351000", sum)
	}

	orders, err := repo.RecentOrders(ctx, 5)
	if err != nil {
		t.Fatalf("RecentOrders: %v", err)
	}
	if len(orders) != 3 || orders[0].OrderID != "o-2" || orders[1].OrderID != "o-1" || orders[1].Total != 350000 {
		t.Errorf("orders = %+v, want o-2, o-1(350000), o-old", orders)
	}

	empty, err := duckdbEmpty(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty.OrderCount != 0 || empty.GMV != 0 {
		t.Errorf("empty summary = %+v, want zero", empty)
	}
}

func duckdbEmpty(ctx context.Context) (query.OrderSummary, error) {
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		return query.OrderSummary{}, err
	}
	defer w.Close()
	return query.NewDuckDBRepository(w.DB()).PlatformOrderSummary(ctx, time.Now().Add(-24*time.Hour))
}
