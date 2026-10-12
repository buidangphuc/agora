package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/store"
)

func TestInMemoryDisputeRepository(t *testing.T) {
	ctx := context.Background()
	repo := store.NewInMemoryDisputeRepository()

	// 1. Create dispute
	d, err := repo.CreateDispute(ctx, store.Dispute{
		OrderID:      "order-100",
		ClaimantID:   "buyer-1",
		DefendantID:  "seller-1",
		Reason:       "Hàng không giống hình ảnh mô tả",
		EvidenceURLs: []string{"https://img.com/1.png"},
	})
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	if d.ID == "" {
		t.Fatal("expected generated dispute ID")
	}
	if d.Status != store.DisputeStatusOpen {
		t.Fatalf("expected status OPEN, got %s", d.Status)
	}

	// 2. Get dispute
	fetched, err := repo.GetDispute(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDispute: %v", err)
	}
	if fetched.Reason != "Hàng không giống hình ảnh mô tả" {
		t.Fatalf("unexpected reason: %s", fetched.Reason)
	}

	// Get nonexistent dispute
	_, err = repo.GetDispute(ctx, "nonexistent-id")
	if !errors.Is(err, store.ErrDisputeNotFound) {
		t.Fatalf("expected ErrDisputeNotFound, got %v", err)
	}

	// 3. Update dispute
	fetched.Status = store.DisputeStatusResolved
	fetched.Resolution = "Đồng ý hoàn tiền 100%"
	updated, err := repo.UpdateDispute(ctx, fetched)
	if err != nil {
		t.Fatalf("UpdateDispute: %v", err)
	}
	if updated.Status != store.DisputeStatusResolved {
		t.Fatalf("expected status RESOLVED, got %s", updated.Status)
	}
	if updated.Resolution != "Đồng ý hoàn tiền 100%" {
		t.Fatalf("unexpected resolution: %s", updated.Resolution)
	}

	// Update nonexistent dispute
	_, err = repo.UpdateDispute(ctx, store.Dispute{ID: "nonexistent-id"})
	if !errors.Is(err, store.ErrDisputeNotFound) {
		t.Fatalf("expected ErrDisputeNotFound, got %v", err)
	}
}
