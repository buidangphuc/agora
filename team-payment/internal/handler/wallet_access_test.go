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

func principalCtx(id string, ptype commonv1.PrincipalType, scopes ...string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: ptype, Scopes: scopes,
	})
}

// TestWalletRPCAccess: no wallet RPC trusts the request seller_id. Only the owner
// (a user principal) may read or pay out; an admin may read but never pay out;
// other users, services and anonymous callers are rejected before any work.
func TestWalletRPCAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ledger := repository.NewInMemoryLedgerRepository()
	wallets := repository.NewInMemoryWalletRepository()
	if _, err := wallets.UpdateWalletBalance(context.Background(), "seller-1", 1_000_000); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	svc := service.NewPaymentService(nil, wallets, nil, logger,
		service.WithLedgerRepo(ledger))
	h := handler.NewPaymentHandler(svc, logger)
	if _, err := ledger.AppendEntry(context.Background(), repository.LedgerEntry{
		SellerID: "seller-1", Type: repository.LedgerTypeOrderSettlement,
		Amount: 1_000_000, Status: repository.LedgerStatusCompleted,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	user := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	reads := map[string]func(ctx context.Context, seller string) error{
		"GetSellerWallet": func(ctx context.Context, s string) error {
			_, err := h.GetSellerWallet(ctx, &paymentv1.GetSellerWalletRequest{SellerId: s})
			return err
		},
		"ListPayoutHistory": func(ctx context.Context, s string) error {
			_, err := h.ListPayoutHistory(ctx, &paymentv1.ListPayoutHistoryRequest{SellerId: s})
			return err
		},
		"GetWalletBalance": func(ctx context.Context, s string) error {
			_, err := h.GetWalletBalance(ctx, &paymentv1.GetWalletBalanceRequest{SellerId: s})
			return err
		},
		"ListLedgerEntries": func(ctx context.Context, s string) error {
			_, err := h.ListLedgerEntries(ctx, &paymentv1.ListLedgerEntriesRequest{SellerId: s})
			return err
		},
	}
	writes := map[string]func(ctx context.Context, seller string) error{
		"RequestPayout": func(ctx context.Context, s string) error {
			_, err := h.RequestPayout(ctx, &paymentv1.RequestPayoutRequest{
				SellerId: s, Amount: 1000, BankCode: "VCB", AccountNumber: "1", AccountName: "A",
			})
			return err
		},
		"RequestWalletPayout": func(ctx context.Context, s string) error {
			_, err := h.RequestWalletPayout(ctx, &paymentv1.RequestWalletPayoutRequest{SellerId: s, Amount: 1000})
			return err
		},
	}

	type tc struct {
		name              string
		ctx               context.Context
		seller            string
		wantRead, wantPay codes.Code
	}
	cases := []tc{
		{"owner explicit", principalCtx("seller-1", user, "seller"), "seller-1", codes.OK, codes.OK},
		{"owner implicit", principalCtx("seller-1", user, "seller"), "", codes.OK, codes.OK},
		{"other user", principalCtx("buyer-9", user, "buyer"), "seller-1", codes.PermissionDenied, codes.PermissionDenied},
		{"admin", principalCtx("admin-1", user, "admin"), "seller-1", codes.OK, codes.PermissionDenied},
		{"service", principalCtx("svc-x", commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, "payment:write"), "seller-1", codes.PermissionDenied, codes.PermissionDenied},
		{"service id equal to seller", principalCtx("seller-1", commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE), "seller-1", codes.PermissionDenied, codes.PermissionDenied},
		{"anonymous", principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS), "seller-1", codes.Unauthenticated, codes.Unauthenticated},
		{"no principal", context.Background(), "seller-1", codes.Unauthenticated, codes.Unauthenticated},
	}
	for _, c := range cases {
		for name, call := range reads {
			if got := status.Code(call(c.ctx, c.seller)); got != c.wantRead {
				t.Errorf("%s/%s: code = %v, want %v", name, c.name, got, c.wantRead)
			}
		}
		for name, call := range writes {
			if got := status.Code(call(c.ctx, c.seller)); got != c.wantPay {
				t.Errorf("%s/%s: code = %v, want %v", name, c.name, got, c.wantPay)
			}
		}
	}
}
