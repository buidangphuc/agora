package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/repository"
)

func TestInMemoryWalletRepository_PayoutRequests(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryWalletRepository()

	req1 := repository.PayoutRequest{
		SellerID:      "seller-1",
		Amount:        100000,
		BankCode:      "VCB",
		AccountNumber: "1234567890",
		AccountName:   "NGUYEN VAN A",
		LedgerEntryID: "ledger-1",
	}

	created, err := repo.CreatePayoutRequest(ctx, req1)
	if err != nil {
		t.Fatalf("CreatePayoutRequest failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty payout ID")
	}
	if created.Status != repository.PayoutStatusPending {
		t.Errorf("want status PENDING, got %v", created.Status)
	}

	// Get payout request
	got, err := repo.GetPayoutRequest(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPayoutRequest failed: %v", err)
	}
	if got.Amount != 100000 || got.BankCode != "VCB" || got.LedgerEntryID != "ledger-1" {
		t.Errorf("mismatched payout data: %+v", got)
	}

	// Get non-existent payout
	_, err = repo.GetPayoutRequest(ctx, "missing-payout")
	if err != repository.ErrPayoutNotFound {
		t.Errorf("expected ErrPayoutNotFound, got %v", err)
	}

	// Create second payout
	time.Sleep(2 * time.Millisecond)
	req2 := repository.PayoutRequest{
		SellerID:      "seller-1",
		Amount:        200000,
		BankCode:      "TCB",
		AccountNumber: "9876543210",
		AccountName:   "NGUYEN VAN A",
	}
	_, err = repo.CreatePayoutRequest(ctx, req2)
	if err != nil {
		t.Fatalf("CreatePayoutRequest 2 failed: %v", err)
	}

	// Create payout for different seller
	req3 := repository.PayoutRequest{
		SellerID:      "seller-2",
		Amount:        50000,
		BankCode:      "MBB",
		AccountNumber: "111222333",
		AccountName:   "TRAN VAN B",
	}
	_, err = repo.CreatePayoutRequest(ctx, req3)
	if err != nil {
		t.Fatalf("CreatePayoutRequest 3 failed: %v", err)
	}

	// List payouts for seller-1
	list1, err := repo.ListPayoutRequestsBySellerID(ctx, "seller-1")
	if err != nil {
		t.Fatalf("ListPayoutRequestsBySellerID failed: %v", err)
	}
	if len(list1) != 2 {
		t.Fatalf("want 2 payouts for seller-1, got %d", len(list1))
	}
	if list1[0].Amount != 200000 {
		t.Errorf("expected newest payout first (200000), got %d", list1[0].Amount)
	}

	// List payouts for seller-2
	list2, err := repo.ListPayoutRequestsBySellerID(ctx, "seller-2")
	if err != nil {
		t.Fatalf("ListPayoutRequestsBySellerID failed: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("want 1 payout for seller-2, got %d", len(list2))
	}

	// List payouts for non-existent seller
	listEmpty, err := repo.ListPayoutRequestsBySellerID(ctx, "seller-none")
	if err != nil {
		t.Fatalf("ListPayoutRequestsBySellerID failed: %v", err)
	}
	if len(listEmpty) != 0 {
		t.Errorf("want 0 payouts, got %d", len(listEmpty))
	}
}
