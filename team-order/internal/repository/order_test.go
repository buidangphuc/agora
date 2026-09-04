package repository_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
)

func TestInMemoryOrderRepository(t *testing.T) {
	repo := repository.NewInMemoryOrderRepository()
	ctx := context.Background()

	t.Run("Create and Get Order", func(t *testing.T) {
		o := repository.Order{
			BuyerID:     "buyer_1",
			SellerID:    "seller_1",
			TotalAmount: 250000,
			Status:      repository.OrderStatusPending,
			Items: []repository.OrderItem{
				{ListingID: "item_1", Quantity: 1, UnitPrice: 250000, Title: "Phone"},
			},
		}

		created, err := repo.CreateOrder(ctx, o)
		if err != nil {
			t.Fatalf("unexpected error creating order: %v", err)
		}
		if created.ID == "" {
			t.Fatalf("expected non-empty order ID")
		}

		got, err := repo.GetOrder(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error getting order: %v", err)
		}
		if got.TotalAmount != 250000 {
			t.Errorf("expected 250000, got %d", got.TotalAmount)
		}

		// List Buyer Orders
		buyerOrders, err := repo.ListBuyerOrders(ctx, "buyer_1", 0)
		if err != nil {
			t.Fatalf("unexpected error listing buyer orders: %v", err)
		}
		if len(buyerOrders) != 1 {
			t.Errorf("expected 1 buyer order, got %d", len(buyerOrders))
		}

		// List Seller Orders
		sellerOrders, err := repo.ListSellerOrders(ctx, "seller_1", 0)
		if err != nil {
			t.Fatalf("unexpected error listing seller orders: %v", err)
		}
		if len(sellerOrders) != 1 {
			t.Errorf("expected 1 seller order, got %d", len(sellerOrders))
		}

		// Update Order Status
		updated, err := repo.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "SPX123456")
		if err != nil {
			t.Fatalf("unexpected error updating status: %v", err)
		}
		if updated.Status != repository.OrderStatusPaid {
			t.Errorf("expected status PAID, got %v", updated.Status)
		}
		if updated.TrackingNumber != "SPX123456" {
			t.Errorf("expected SPX123456, got %s", updated.TrackingNumber)
		}
	})
}
