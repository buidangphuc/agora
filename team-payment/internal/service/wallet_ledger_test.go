package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

func newLedgerService() (*service.PaymentService, *repository.InMemoryLedgerRepository) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ledger := repository.NewInMemoryLedgerRepository()
	svc := service.NewPaymentService(nil, nil, nil, logger, service.WithLedgerRepo(ledger))
	return svc, ledger
}

// Test 1: credit then balance = sum of ledger.
func TestWalletService_CreditThenBalance(t *testing.T) {
	ctx := context.Background()
	svc, _ := newLedgerService()

	if _, err := svc.CreditWallet(ctx, "seller-1", 500000, repository.LedgerTypeOrderSettlement); err != nil {
		t.Fatalf("CreditWallet failed: %v", err)
	}
	if _, err := svc.CreditWallet(ctx, "seller-1", 300000, repository.LedgerTypeOrderSettlement); err != nil {
		t.Fatalf("CreditWallet 2 failed: %v", err)
	}

	balance, err := svc.GetWalletBalance(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetWalletBalance failed: %v", err)
	}
	if balance != 800000 {
		t.Errorf("want balance 800000, got %d", balance)
	}

	// Empty seller has zero balance, no panic.
	empty, err := svc.GetWalletBalance(ctx, "seller-none")
	if err != nil {
		t.Fatalf("GetWalletBalance empty failed: %v", err)
	}
	if empty != 0 {
		t.Errorf("want balance 0 for unknown seller, got %d", empty)
	}
}

// Test 2: RequestWalletPayout records a PENDING debit that lowers the balance.
func TestWalletService_RequestWalletPayout_PendingDebit(t *testing.T) {
	ctx := context.Background()
	svc, _ := newLedgerService()

	if _, err := svc.CreditWallet(ctx, "seller-1", 1000000, repository.LedgerTypeOrderSettlement); err != nil {
		t.Fatalf("CreditWallet failed: %v", err)
	}

	entry, err := svc.RequestWalletPayout(ctx, "seller-1", 400000)
	if err != nil {
		t.Fatalf("RequestWalletPayout failed: %v", err)
	}
	if entry.Status != repository.LedgerStatusPending {
		t.Errorf("want status PENDING, got %s", entry.Status)
	}
	if entry.Type != repository.LedgerTypePayout {
		t.Errorf("want type PAYOUT, got %s", entry.Type)
	}
	if entry.Amount != -400000 {
		t.Errorf("want debit amount -400000, got %d", entry.Amount)
	}

	// Balance reflects the pending debit (balance = sum of ledger).
	balance, err := svc.GetWalletBalance(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetWalletBalance failed: %v", err)
	}
	if balance != 600000 {
		t.Errorf("want balance 600000 after payout, got %d", balance)
	}

	// Edge: payout exceeding balance is rejected, no entry recorded.
	if _, err := svc.RequestWalletPayout(ctx, "seller-1", 999999999); !errors.Is(err, repository.ErrInsufficientBalance) {
		t.Errorf("want ErrInsufficientBalance, got %v", err)
	}

	// Edge: non-positive amount rejected.
	if _, err := svc.RequestWalletPayout(ctx, "seller-1", 0); !errors.Is(err, repository.ErrInvalidAmount) {
		t.Errorf("want ErrInvalidAmount, got %v", err)
	}
}

// Test 3: list pagination + cross-seller isolation.
func TestWalletService_ListLedger_PaginationAndIsolation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newLedgerService()

	// seller-1 gets 5 entries, seller-2 gets 2 (isolation guard).
	for i := 0; i < 5; i++ {
		if _, err := svc.CreditWallet(ctx, "seller-1", int64(1000*(i+1)), repository.LedgerTypeOrderSettlement); err != nil {
			t.Fatalf("seed seller-1 failed: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.CreditWallet(ctx, "seller-2", 5000, repository.LedgerTypeOrderSettlement); err != nil {
			t.Fatalf("seed seller-2 failed: %v", err)
		}
	}

	// Page 1: size 2.
	page1, next1, total, err := svc.ListLedgerEntries(ctx, "seller-1", "", 2)
	if err != nil {
		t.Fatalf("ListLedgerEntries page1 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("want total 5, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("want 2 entries on page1, got %d", len(page1))
	}
	if next1 == "" {
		t.Fatal("expected a next cursor after page1")
	}
	// Newest first: last-seeded amount (5000) leads.
	if page1[0].Amount != 5000 {
		t.Errorf("want newest entry (5000) first, got %d", page1[0].Amount)
	}
	for _, e := range page1 {
		if e.SellerID != "seller-1" {
			t.Errorf("cross-seller leak: got entry for %s", e.SellerID)
		}
	}

	// Page 2 via cursor.
	page2, next2, _, err := svc.ListLedgerEntries(ctx, "seller-1", next1, 2)
	if err != nil {
		t.Fatalf("ListLedgerEntries page2 failed: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("want 2 entries on page2, got %d", len(page2))
	}

	// Page 3: remainder, cursor exhausted.
	page3, next3, _, err := svc.ListLedgerEntries(ctx, "seller-1", next2, 2)
	if err != nil {
		t.Fatalf("ListLedgerEntries page3 failed: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("want 1 entry on page3, got %d", len(page3))
	}
	if next3 != "" {
		t.Errorf("want empty next cursor at end, got %q", next3)
	}

	// Cross-seller isolation: seller-2 only sees its own 2 entries.
	s2, _, s2Total, err := svc.ListLedgerEntries(ctx, "seller-2", "", 50)
	if err != nil {
		t.Fatalf("ListLedgerEntries seller-2 failed: %v", err)
	}
	if s2Total != 2 || len(s2) != 2 {
		t.Errorf("want seller-2 total/len 2, got total=%d len=%d", s2Total, len(s2))
	}

	// Edge: malformed cursor is rejected.
	if _, _, _, err := svc.ListLedgerEntries(ctx, "seller-1", "!!!not-base64!!!", 2); !errors.Is(err, service.ErrInvalidPageToken) {
		t.Errorf("want ErrInvalidPageToken, got %v", err)
	}
}

func TestWalletService_NotConfigured(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewPaymentService(nil, nil, nil, logger) // no ledger repo

	if _, err := svc.GetWalletBalance(ctx, "seller-1"); !errors.Is(err, service.ErrLedgerNotConfigured) {
		t.Errorf("want ErrLedgerNotConfigured, got %v", err)
	}
}
