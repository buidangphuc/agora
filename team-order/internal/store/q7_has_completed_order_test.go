package store_test

import (
	"context"
	"testing"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

func q7NewOrder(ctx context.Context, t *testing.T, repo store.OrderRepository, buyer string) store.Order {
	t.Helper()
	o, err := repo.CreateOrder(ctx, store.Order{
		BuyerID: buyer, SellerID: "q7-seller", TotalAmount: 120000, Currency: "VND",
		PaymentMethod: int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO),
		Items:         []store.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: 120000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func q7Complete(ctx context.Context, t *testing.T, repo store.OrderRepository, o store.Order, by orderv1.CompletedBy) {
	t.Helper()
	shipped := rcShip(ctx, t, repo, o)
	if _, err := repo.UpdateOrderStatusFrom(ctx, shipped.ID, store.OrderStatusCompleted, rcFromShipped, "", store.WithCompletedBy(int32(by))); err != nil {
		t.Fatal(err)
	}
}

func q7Has(ctx context.Context, t *testing.T, repo store.OrderRepository, buyer string) bool {
	t.Helper()
	got, err := repo.HasCompletedOrder(ctx, buyer)
	if err != nil {
		t.Fatalf("HasCompletedOrder(%s): %v", buyer, err)
	}
	return got
}

func runQ7HasCompletedOrderContract(t *testing.T, mk func(t *testing.T) store.OrderRepository) {
	ctx := context.Background()

	t.Run("TestQ7_BuyerWithACompletedOrder", func(t *testing.T) {
		repo := mk(t)
		a := uid("buyer-a")
		q7Complete(ctx, t, repo, q7NewOrder(ctx, t, repo, a), orderv1.CompletedBy_COMPLETED_BY_BUYER)
		if !q7Has(ctx, t, repo, a) {
			t.Fatal("want true for a buyer with a COMPLETED order")
		}
	})

	t.Run("TestQ7_BuyerWithACompletedAndACancelledOrder", func(t *testing.T) {
		repo := mk(t)
		a := uid("buyer-a")
		q7Complete(ctx, t, repo, q7NewOrder(ctx, t, repo, a), orderv1.CompletedBy_COMPLETED_BY_BUYER)
		c := q7NewOrder(ctx, t, repo, a)
		if _, err := repo.UpdateOrderStatusFrom(ctx, c.ID, store.OrderStatusCancelled, fromPendingPaid, ""); err != nil {
			t.Fatal(err)
		}
		if !q7Has(ctx, t, repo, a) {
			t.Fatal("want true when one order is COMPLETED and another CANCELLED")
		}
	})

	t.Run("TestQ7_CompletedBySellerOrSystemCounts", func(t *testing.T) {
		for name, by := range map[string]orderv1.CompletedBy{
			"seller": orderv1.CompletedBy_COMPLETED_BY_SELLER,
			"system": orderv1.CompletedBy_COMPLETED_BY_SYSTEM,
		} {
			t.Run(name, func(t *testing.T) {
				repo := mk(t)
				a := uid("buyer-a")
				q7Complete(ctx, t, repo, q7NewOrder(ctx, t, repo, a), by)
				if !q7Has(ctx, t, repo, a) {
					t.Fatalf("want true when completed by %s", name)
				}
			})
		}
	})

	t.Run("TestQ7_BuyerWithOnlyNonCompletedOrders", func(t *testing.T) {
		repo := mk(t)
		b := uid("buyer-b")
		q7NewOrder(ctx, t, repo, b)
		paid := q7NewOrder(ctx, t, repo, b)
		if _, err := repo.UpdateOrderStatusFrom(ctx, paid.ID, store.OrderStatusPaid, fromPending, ""); err != nil {
			t.Fatal(err)
		}
		rcShip(ctx, t, repo, q7NewOrder(ctx, t, repo, b))
		cancelled := q7NewOrder(ctx, t, repo, b)
		if _, err := repo.UpdateOrderStatusFrom(ctx, cancelled.ID, store.OrderStatusCancelled, fromPendingPaid, ""); err != nil {
			t.Fatal(err)
		}
		if q7Has(ctx, t, repo, b) {
			t.Fatal("want false when no order is COMPLETED")
		}
	})

	t.Run("TestQ7_BuyerWithNoOrders", func(t *testing.T) {
		repo := mk(t)
		got, err := repo.HasCompletedOrder(ctx, uid("buyer-c"))
		if err != nil {
			t.Fatalf("a buyer with no orders must not be an error: %v", err)
		}
		if got {
			t.Fatal("want false for a buyer with no orders")
		}
	})

	t.Run("TestQ7_AnotherBuyersCompletedOrderDoesNotCount", func(t *testing.T) {
		repo := mk(t)
		a, b := uid("buyer-a"), uid("buyer-b")
		q7Complete(ctx, t, repo, q7NewOrder(ctx, t, repo, a), orderv1.CompletedBy_COMPLETED_BY_BUYER)
		q7NewOrder(ctx, t, repo, b)
		if q7Has(ctx, t, repo, b) {
			t.Fatal("want false for B when only A has a COMPLETED order")
		}
		if !q7Has(ctx, t, repo, a) {
			t.Fatal("want true for A")
		}
	})

	t.Run("TestQ7_StatusChangeIsVisibleOnTheNextCall", func(t *testing.T) {
		repo := mk(t)
		b := uid("buyer-b")
		shipped := rcShip(ctx, t, repo, q7NewOrder(ctx, t, repo, b))
		if q7Has(ctx, t, repo, b) {
			t.Fatal("want false before the SHIPPED order completes")
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, shipped.ID, store.OrderStatusCompleted, rcFromShipped, "", store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_BUYER))); err != nil {
			t.Fatal(err)
		}
		if !q7Has(ctx, t, repo, b) {
			t.Fatal("want true on the call after completion")
		}
	})
}

func TestQ7HasCompletedOrder_InMemory(t *testing.T) {
	runQ7HasCompletedOrderContract(t, func(_ *testing.T) store.OrderRepository {
		return fake.NewInMemoryOrderRepository(rcBaseOpts()...)
	})
}

func TestQ7HasCompletedOrder_Postgres(t *testing.T) {
	pool := pgPool(t)
	runQ7HasCompletedOrderContract(t, func(t *testing.T) store.OrderRepository {
		opts := append(rcBaseOpts(), store.WithCancelledOutbox(BuildCancelledOutboxRow))
		return store.NewPostgresOrderRepository(pool, newOutbox(t, pool), opts...)
	})
}
