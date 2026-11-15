package order_test

import (
	"testing"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

func seedSellerOrders(t *testing.T, repo *fake.InMemoryOrderRepository, seller string, st store.OrderStatus, title string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := repo.CreateOrder(t.Context(), store.Order{
			BuyerID: "buyer", SellerID: seller, Status: st,
			Items: []store.OrderItem{{ListingID: "l", Title: title, Quantity: 1, UnitPrice: 1}},
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// Scenario: Legacy call without any field
func TestSellerOrdersService_LegacyCallWithoutAnyFieldReturnsEveryOrder(t *testing.T) {
	repo := fake.NewInMemoryOrderRepository()
	seedSellerOrders(t, repo, "sA", store.OrderStatusPaid, "x", 6)
	seedSellerOrders(t, repo, "sB", store.OrderStatusPaid, "x", 2)
	svc := newSvc(t, repo, nil, nil, nil, nil, nil, nil)

	res, err := svc.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.Orders) != 6 || res.Next != nil || res.Total != 6 {
		t.Fatalf("orders=%d next=%v total=%d, want 6 nil 6", len(res.Orders), res.Next, res.Total)
	}
	for _, o := range res.Orders {
		if o.SellerID != "sA" {
			t.Fatalf("foreign order %s returned", o.ID)
		}
	}
	if len(res.Counts) != 5 || res.Counts[1].Status != store.OrderStatusPaid || res.Counts[1].Count != 6 {
		t.Fatalf("counts = %+v", res.Counts)
	}
}

// Scenario: statuses wins over status_filter (service takes the resolved Statuses list)
func TestSellerOrdersService_StatusesFilterTheResultAndCountsIgnoreIt(t *testing.T) {
	repo := fake.NewInMemoryOrderRepository()
	seedSellerOrders(t, repo, "sA", store.OrderStatusPaid, "x", 3)
	seedSellerOrders(t, repo, "sA", store.OrderStatusShipped, "x", 2)
	seedSellerOrders(t, repo, "sA", store.OrderStatusCancelled, "x", 1)
	svc := newSvc(t, repo, nil, nil, nil, nil, nil, nil)

	res, err := svc.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Statuses: []store.OrderStatus{store.OrderStatusPaid}, Paged: true})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.Orders) != 3 || res.Total != 3 {
		t.Fatalf("orders=%d total=%d, want 3 3", len(res.Orders), res.Total)
	}
	want := []int64{0, 3, 2, 0, 1}
	for i, c := range res.Counts {
		if c.Count != want[i] {
			t.Fatalf("counts = %+v, want %v", res.Counts, want)
		}
	}
}

// Scenario: Another seller's orders are never returned (query through the service)
func TestSellerOrdersService_QueryNeverReachesAnotherSellersOrders(t *testing.T) {
	repo := fake.NewInMemoryOrderRepository()
	seedSellerOrders(t, repo, "sA", store.OrderStatusPaid, "Ao thun doc nhat", 1)
	svc := newSvc(t, repo, nil, nil, nil, nil, nil, nil)

	res, err := svc.ListSellerOrders(t.Context(), "sB", store.SellerOrdersFilter{Query: "Ao thun doc nhat", Paged: true})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.Orders) != 0 || res.Total != 0 {
		t.Fatalf("orders=%d total=%d, want 0 0", len(res.Orders), res.Total)
	}
}
