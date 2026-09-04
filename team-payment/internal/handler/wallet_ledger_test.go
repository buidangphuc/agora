package handler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/interceptor"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

func setupLedgerHandler() (*handler.PaymentHandler, *repository.InMemoryLedgerRepository) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ledger := repository.NewInMemoryLedgerRepository()
	svc := service.NewPaymentService(nil, nil, nil, logger, service.WithLedgerRepo(ledger))
	return handler.NewPaymentHandler(svc, logger), ledger
}

func authCtx(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id:     id,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"payment:read", "payment:write"},
	})
}

func TestPaymentHandler_WalletLedger(t *testing.T) {
	h, ledger := setupLedgerHandler()
	ctx := authCtx("seller-1")

	// Seed a credit directly via the repo, then defaulting seller_id to the principal.
	if _, err := ledger.AppendEntry(ctx, repository.LedgerEntry{
		SellerID: "seller-1", Type: repository.LedgerTypeOrderSettlement,
		Amount: 1000000, Status: repository.LedgerStatusCompleted,
	}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	// GetWalletBalance defaults to the principal's own id.
	balResp, err := h.GetWalletBalance(ctx, &paymentv1.GetWalletBalanceRequest{})
	if err != nil {
		t.Fatalf("GetWalletBalance failed: %v", err)
	}
	if balResp.GetBalance() != 1000000 {
		t.Errorf("want balance 1000000, got %d", balResp.GetBalance())
	}

	// RequestWalletPayout books a PENDING debit.
	payoutResp, err := h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{Amount: 250000})
	if err != nil {
		t.Fatalf("RequestWalletPayout failed: %v", err)
	}
	if payoutResp.GetEntry().GetStatus() != repository.LedgerStatusPending {
		t.Errorf("want PENDING entry, got %s", payoutResp.GetEntry().GetStatus())
	}
	if payoutResp.GetEntry().GetAmount() != -250000 {
		t.Errorf("want debit -250000, got %d", payoutResp.GetEntry().GetAmount())
	}

	// ListLedgerEntries returns both entries with a total.
	listResp, err := h.ListLedgerEntries(ctx, &paymentv1.ListLedgerEntriesRequest{
		Page: &commonv1.PageRequest{PageSize: 10},
	})
	if err != nil {
		t.Fatalf("ListLedgerEntries failed: %v", err)
	}
	if listResp.GetPage().GetTotal() != 2 {
		t.Errorf("want total 2, got %d", listResp.GetPage().GetTotal())
	}
	if len(listResp.GetEntries()) != 2 {
		t.Errorf("want 2 entries, got %d", len(listResp.GetEntries()))
	}
}

func TestPaymentHandler_WalletLedger_AnonymousRejected(t *testing.T) {
	h, _ := setupLedgerHandler()

	// No principal in context => Unauthenticated, no panic.
	_, err := h.GetWalletBalance(context.Background(), &paymentv1.GetWalletBalanceRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want Unauthenticated, got %v", err)
	}

	_, err = h.RequestWalletPayout(context.Background(), &paymentv1.RequestWalletPayoutRequest{Amount: 100})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want Unauthenticated for payout, got %v", err)
	}

	_, err = h.ListLedgerEntries(context.Background(), &paymentv1.ListLedgerEntriesRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want Unauthenticated for list, got %v", err)
	}
}

func TestPaymentHandler_WalletLedger_CrossSellerIsolation(t *testing.T) {
	h, ledger := setupLedgerHandler()
	ctx := authCtx("seller-1")

	_, _ = ledger.AppendEntry(ctx, repository.LedgerEntry{SellerID: "seller-1", Type: repository.LedgerTypeOrderSettlement, Amount: 100, Status: repository.LedgerStatusCompleted})
	_, _ = ledger.AppendEntry(ctx, repository.LedgerEntry{SellerID: "seller-2", Type: repository.LedgerTypeOrderSettlement, Amount: 999, Status: repository.LedgerStatusCompleted})

	// Explicit seller_id scopes the query; seller-1 never sees seller-2's entries.
	resp, err := h.ListLedgerEntries(ctx, &paymentv1.ListLedgerEntriesRequest{
		SellerId: "seller-1",
		Page:     &commonv1.PageRequest{PageSize: 10},
	})
	if err != nil {
		t.Fatalf("ListLedgerEntries failed: %v", err)
	}
	if resp.GetPage().GetTotal() != 1 {
		t.Errorf("want total 1 for seller-1, got %d", resp.GetPage().GetTotal())
	}
	for _, e := range resp.GetEntries() {
		if e.GetSellerId() != "seller-1" {
			t.Errorf("cross-seller leak: %s", e.GetSellerId())
		}
	}
}
