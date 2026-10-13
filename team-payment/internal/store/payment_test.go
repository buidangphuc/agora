package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-payment/internal/store"
)

func TestInMemoryPaymentRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := store.NewInMemoryPaymentRepository()

	tx := store.PaymentTransaction{
		OrderID:  "order-1",
		BuyerID:  "buyer-1",
		Amount:   150000,
		Currency: "VND",
		Method:   store.PaymentMethodMockMoMo,
		Status:   store.PaymentStatusPending,
	}

	created, err := repo.CreateTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("CreateTransaction failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Currency != "VND" {
		t.Fatalf("expected VND currency, got %s", created.Currency)
	}
	if created.Status != store.PaymentStatusPending {
		t.Fatalf("expected PENDING status, got %v", created.Status)
	}

	// Get by ID
	byID, err := repo.GetTransaction(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if byID.OrderID != "order-1" || byID.Amount != 150000 {
		t.Fatalf("mismatched transaction data: %+v", byID)
	}

	// Get by Order ID
	byOrder, err := repo.GetTransactionByOrderID(ctx, "order-1")
	if err != nil {
		t.Fatalf("GetTransactionByOrderID failed: %v", err)
	}
	if byOrder.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, byOrder.ID)
	}

	// Get non-existent
	_, err = repo.GetTransaction(ctx, "non-existent")
	if !errors.Is(err, store.ErrTransactionNotFound) {
		t.Fatalf("expected ErrTransactionNotFound, got %v", err)
	}

	_, err = repo.GetTransactionByOrderID(ctx, "non-existent-order")
	if !errors.Is(err, store.ErrTransactionNotFound) {
		t.Fatalf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestInMemoryPaymentRepository_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	repo := store.NewInMemoryPaymentRepository()

	tx := store.PaymentTransaction{
		OrderID: "order-2",
		BuyerID: "buyer-2",
		Amount:  500000,
	}
	created, err := repo.CreateTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("CreateTransaction failed: %v", err)
	}

	// Update status
	updated, err := repo.UpdateTransactionStatus(ctx, created.ID, store.PaymentStatusPaid, "gateway-ref-123")
	if err != nil {
		t.Fatalf("UpdateTransactionStatus failed: %v", err)
	}
	if updated.Status != store.PaymentStatusPaid {
		t.Errorf("expected PAID status")
	}

	// Update non-existent
	_, err = repo.UpdateTransactionStatus(ctx, "non-existent", store.PaymentStatusPaid, "")
	if !errors.Is(err, store.ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound")
	}
}
