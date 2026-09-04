package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/repository"
)

func TestInMemoryWalletRepository_GetOrCreateWallet(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryWalletRepository()

	// 1. First get/create initializes wallet
	w1, err := repo.GetOrCreateWallet(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetOrCreateWallet failed: %v", err)
	}
	if w1.ID == "" {
		t.Error("expected non-empty wallet ID")
	}
	if w1.SellerID != "seller-1" {
		t.Errorf("want seller-1, got %s", w1.SellerID)
	}
	if w1.Balance != 0 {
		t.Errorf("want balance 0, got %d", w1.Balance)
	}
	if w1.Currency != "VND" {
		t.Errorf("want currency VND, got %s", w1.Currency)
	}

	// 2. Second get/create returns existing wallet
	w2, err := repo.GetOrCreateWallet(ctx, "seller-1")
	if err != nil {
		t.Fatalf("second GetOrCreateWallet failed: %v", err)
	}
	if w2.ID != w1.ID {
		t.Errorf("expected same wallet ID %s, got %s", w1.ID, w2.ID)
	}

	// 3. GetWalletBySellerID
	bySeller, err := repo.GetWalletBySellerID(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetWalletBySellerID failed: %v", err)
	}
	if bySeller.ID != w1.ID {
		t.Errorf("expected wallet ID %s, got %s", w1.ID, bySeller.ID)
	}

	// 4. Get non-existent wallet
	_, err = repo.GetWalletBySellerID(ctx, "seller-non-existent")
	if err != repository.ErrWalletNotFound {
		t.Errorf("expected ErrWalletNotFound, got %v", err)
	}
}

func TestInMemoryWalletRepository_UpdateWalletBalance(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryWalletRepository()

	// 1. Credit wallet
	w, err := repo.UpdateWalletBalance(ctx, "seller-1", 500000)
	if err != nil {
		t.Fatalf("UpdateWalletBalance credit failed: %v", err)
	}
	if w.Balance != 500000 {
		t.Errorf("want balance 500000, got %d", w.Balance)
	}

	// 2. Additional credit
	w, err = repo.UpdateWalletBalance(ctx, "seller-1", 200000)
	if err != nil {
		t.Fatalf("UpdateWalletBalance credit 2 failed: %v", err)
	}
	if w.Balance != 700000 {
		t.Errorf("want balance 700000, got %d", w.Balance)
	}

	// 3. Valid debit
	w, err = repo.UpdateWalletBalance(ctx, "seller-1", -300000)
	if err != nil {
		t.Fatalf("UpdateWalletBalance debit failed: %v", err)
	}
	if w.Balance != 400000 {
		t.Errorf("want balance 400000, got %d", w.Balance)
	}

	// 4. Insufficient balance debit
	_, err = repo.UpdateWalletBalance(ctx, "seller-1", -500000)
	if err != repository.ErrInsufficientBalance {
		t.Errorf("expected ErrInsufficientBalance, got %v", err)
	}

	// Check balance remained unchanged after failed debit
	w, err = repo.GetWalletBySellerID(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetWalletBySellerID failed: %v", err)
	}
	if w.Balance != 400000 {
		t.Errorf("want balance 400000, got %d", w.Balance)
	}
}

func TestInMemoryWalletRepository_PayoutRequests(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryWalletRepository()

	req1 := repository.PayoutRequest{
		SellerID:      "seller-1",
		Amount:        100000,
		BankCode:      "VCB",
		AccountNumber: "1234567890",
		AccountName:   "NGUYEN VAN A",
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
	if got.Amount != 100000 || got.BankCode != "VCB" {
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

func TestInMemoryWalletRepository_WalletTransactions(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryWalletRepository()

	tx1 := repository.WalletTransaction{
		WalletID:    "wallet-1",
		Amount:      500000,
		Type:        repository.WalletTxTypeOrderSettlement,
		ReferenceID: "order-100",
	}

	created1, err := repo.CreateWalletTransaction(ctx, tx1)
	if err != nil {
		t.Fatalf("CreateWalletTransaction failed: %v", err)
	}
	if created1.ID == "" {
		t.Fatal("expected non-empty tx ID")
	}

	tx2 := repository.WalletTransaction{
		WalletID:    "wallet-1",
		Amount:      -200000,
		Type:        repository.WalletTxTypePayout,
		ReferenceID: "payout-200",
	}
	_, err = repo.CreateWalletTransaction(ctx, tx2)
	if err != nil {
		t.Fatalf("CreateWalletTransaction 2 failed: %v", err)
	}

	tx3 := repository.WalletTransaction{
		WalletID:    "wallet-2",
		Amount:      300000,
		Type:        repository.WalletTxTypeOrderSettlement,
		ReferenceID: "order-300",
	}
	_, err = repo.CreateWalletTransaction(ctx, tx3)
	if err != nil {
		t.Fatalf("CreateWalletTransaction 3 failed: %v", err)
	}

	// List for wallet-1
	list1, err := repo.ListWalletTransactions(ctx, "wallet-1")
	if err != nil {
		t.Fatalf("ListWalletTransactions failed: %v", err)
	}
	if len(list1) != 2 {
		t.Fatalf("want 2 transactions for wallet-1, got %d", len(list1))
	}

	// List for wallet-2
	list2, err := repo.ListWalletTransactions(ctx, "wallet-2")
	if err != nil {
		t.Fatalf("ListWalletTransactions 2 failed: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("want 1 transaction for wallet-2, got %d", len(list2))
	}
}
