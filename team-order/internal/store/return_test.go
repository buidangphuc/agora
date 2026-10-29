package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

func TestInMemoryReturnRepository(t *testing.T) {
	ctx := context.Background()
	repo := fake.NewInMemoryReturnRepository()

	// 1. Create Return
	req := store.OrderReturn{
		OrderID:      "order-123",
		BuyerID:      "buyer-456",
		SellerID:     "seller-789",
		Reason:       "Item damaged upon delivery",
		RefundAmount: 250000,
	}

	created, err := repo.CreateReturn(ctx, req)
	if err != nil {
		t.Fatalf("CreateReturn failed: %v", err)
	}
	if created.ID == "" {
		t.Errorf("expected generated ID, got empty")
	}
	if created.Status != store.ReturnStatusPending {
		t.Errorf("expected status PENDING (%d), got %d", store.ReturnStatusPending, created.Status)
	}

	// 2. Get Return
	got, err := repo.GetReturn(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetReturn failed: %v", err)
	}
	if got.OrderID != "order-123" || got.BuyerID != "buyer-456" {
		t.Errorf("unexpected return data: %+v", got)
	}

	// 3. Get Return by Order ID
	byOrder, err := repo.GetReturnByOrderID(ctx, "order-123")
	if err != nil {
		t.Fatalf("GetReturnByOrderID failed: %v", err)
	}
	if byOrder.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, byOrder.ID)
	}

	// 4. List by Buyer & Seller
	buyerList, err := repo.ListReturnsByBuyer(ctx, "buyer-456")
	if err != nil || len(buyerList) != 1 {
		t.Errorf("ListReturnsByBuyer expected 1 item, got %d, err: %v", len(buyerList), err)
	}

	sellerList, err := repo.ListReturnsBySeller(ctx, "seller-789")
	if err != nil || len(sellerList) != 1 {
		t.Errorf("ListReturnsBySeller expected 1 item, got %d, err: %v", len(sellerList), err)
	}

	// 5. Update Status
	updated, err := repo.TransitionReturn(ctx, created.ID, store.ReturnStatusPending, store.ReturnStatusApproved)
	if err != nil {
		t.Fatalf("TransitionReturn failed: %v", err)
	}
	if updated.Status != store.ReturnStatusApproved {
		t.Errorf("expected APPROVED, got %d", updated.Status)
	}

	// 6. Not Found cases
	if _, err := repo.GetReturn(ctx, "non-existent"); !errors.Is(err, store.ErrReturnNotFound) {
		t.Errorf("expected ErrReturnNotFound, got %v", err)
	}
	if _, err := repo.GetReturnByOrderID(ctx, "non-existent"); !errors.Is(err, store.ErrReturnNotFound) {
		t.Errorf("expected ErrReturnNotFound, got %v", err)
	}
}
