package handler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/interceptor"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

type mockOrderClient struct {
	orders map[string]*orderv1.Order
}

func (m *mockOrderClient) GetOrder(_ context.Context, req *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	if o, ok := m.orders[req.GetId()]; ok {
		return &orderv1.GetOrderResponse{Order: o}, nil
	}
	return nil, service.ErrOrderNotFound
}

func (m *mockOrderClient) UpdateOrderStatus(_ context.Context, req *orderv1.UpdateOrderStatusRequest, _ ...grpc.CallOption) (*orderv1.UpdateOrderStatusResponse, error) {
	if o, ok := m.orders[req.GetId()]; ok {
		o.Status = req.GetStatus()
		return &orderv1.UpdateOrderStatusResponse{Order: o}, nil
	}
	return nil, service.ErrOrderNotFound
}

func setupHandlerTest() (*handler.PaymentHandler, *repository.InMemoryPaymentRepository, *repository.InMemoryLedgerRepository, *mockOrderClient) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	paymentRepo := repository.NewInMemoryPaymentRepository()
	walletRepo := repository.NewInMemoryWalletRepository()
	ledger := repository.NewInMemoryLedgerRepository()
	orderClient := &mockOrderClient{
		orders: map[string]*orderv1.Order{
			"order-1": {
				Id:          "order-1",
				BuyerId:     "buyer-1",
				TotalAmount: 100000,
				Currency:    "VND",
				Status:      orderv1.OrderStatus_ORDER_STATUS_PENDING,
			},
		},
	}
	svc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger, service.WithLedgerRepo(ledger))
	h := handler.NewPaymentHandler(svc, logger, handler.WithMockPayments(true))
	return h, paymentRepo, ledger, orderClient
}

func TestPaymentHandler_Payments(t *testing.T) {
	h, _, _, _ := setupHandlerTest()

	principal := &commonv1.Principal{
		Id:     "buyer-1",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"payment:write", "payment:read"},
	}
	ctx := interceptor.ContextWithPrincipal(context.Background(), principal)

	t.Run("CreatePayment", func(t *testing.T) {
		res, err := h.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
			OrderId: "order-1",
			Method:  paymentv1.PaymentMethod_PAYMENT_METHOD_COD,
		})
		if err != nil {
			t.Fatalf("unexpected error creating payment: %v", err)
		}
		if res.Transaction.OrderId != "order-1" {
			t.Errorf("expected order-1, got %s", res.Transaction.OrderId)
		}

		t.Run("GetPayment by ID", func(t *testing.T) {
			getRes, err := h.GetPayment(ctx, &paymentv1.GetPaymentRequest{
				Id: res.Transaction.Id,
			})
			if err != nil {
				t.Fatalf("unexpected error getting payment: %v", err)
			}
			if getRes.Transaction.Id != res.Transaction.Id {
				t.Errorf("expected transaction ID %s, got %s", res.Transaction.Id, getRes.Transaction.Id)
			}
		})

		t.Run("GetPayment by OrderID", func(t *testing.T) {
			getRes, err := h.GetPayment(ctx, &paymentv1.GetPaymentRequest{
				OrderId: "order-1",
			})
			if err != nil {
				t.Fatalf("unexpected error getting payment: %v", err)
			}
			if getRes.Transaction.OrderId != "order-1" {
				t.Errorf("expected order-1, got %s", getRes.Transaction.OrderId)
			}
		})

		t.Run("ProcessMockPayment success", func(t *testing.T) {
			procRes, err := h.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{
				TransactionId:   res.Transaction.Id,
				SimulateSuccess: true,
			})
			if err != nil {
				t.Fatalf("unexpected error processing payment: %v", err)
			}
			if !procRes.Success {
				t.Errorf("expected success true")
			}
		})
	})
}

func TestPaymentHandler_CreatePaymentOwnership(t *testing.T) {
	h, _, _, _ := setupHandlerTest()
	req := &paymentv1.CreatePaymentRequest{OrderId: "order-1", Method: paymentv1.PaymentMethod_PAYMENT_METHOD_COD}
	user := commonv1.PrincipalType_PRINCIPAL_TYPE_USER

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", context.Background(), codes.Unauthenticated},
		{"other user", principalCtx("buyer-2", user), codes.PermissionDenied},
		{"seller of the order", principalCtx("seller-1", user), codes.PermissionDenied},
		{"owner", principalCtx("buyer-1", user), codes.OK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.CreatePayment(tc.ctx, req)
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", got, tc.want, err)
			}
		})
	}
}

// TestPaymentHandler_TransactionAccess: GetPayment and ProcessMockPayment trust
// neither the request ids nor the metadata alone; only the order's buyer or an
// admin may read or settle a transaction.
func TestPaymentHandler_TransactionAccess(t *testing.T) {
	user := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	svc := commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", context.Background(), codes.Unauthenticated},
		{"gateway anonymous principal", principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS), codes.Unauthenticated},
		{"other user", principalCtx("buyer-2", user), codes.PermissionDenied},
		{"order's seller", principalCtx("seller-1", user), codes.PermissionDenied},
		{"service principal", principalCtx("buyer-1", svc), codes.PermissionDenied},
		{"owner", principalCtx("buyer-1", user), codes.OK},
		{"admin", principalCtx("admin-1", user, "admin"), codes.OK},
	}
	for _, tc := range tests {
		t.Run("GetPayment/"+tc.name, func(t *testing.T) {
			h, repo, _, _ := setupHandlerTest()
			seeded := seedTx(t, repo)
			_, err := h.GetPayment(tc.ctx, &paymentv1.GetPaymentRequest{Id: seeded.ID})
			if got := status.Code(err); got != tc.want {
				t.Fatalf("by id: code = %v, want %v (err=%v)", got, tc.want, err)
			}
			_, err = h.GetPayment(tc.ctx, &paymentv1.GetPaymentRequest{OrderId: "order-1"})
			if got := status.Code(err); got != tc.want {
				t.Fatalf("by order: code = %v, want %v (err=%v)", got, tc.want, err)
			}
		})
		t.Run("ProcessMockPayment/"+tc.name, func(t *testing.T) {
			h, repo, _, _ := setupHandlerTest()
			seeded := seedTx(t, repo)
			_, err := h.ProcessMockPayment(tc.ctx, &paymentv1.ProcessMockPaymentRequest{
				TransactionId: seeded.ID, SimulateSuccess: true,
			})
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", got, tc.want, err)
			}
			after, gerr := repo.GetTransaction(context.Background(), seeded.ID)
			if gerr != nil {
				t.Fatalf("reload: %v", gerr)
			}
			settled := after.Status == repository.PaymentStatusPaid
			if settled != (tc.want == codes.OK) {
				t.Fatalf("settled = %v for %s", settled, tc.name)
			}
		})
	}
}

func seedTx(t *testing.T, repo *repository.InMemoryPaymentRepository) repository.PaymentTransaction {
	t.Helper()
	tx, err := repo.CreateTransaction(context.Background(), repository.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 100000, Currency: "VND",
		Method: repository.PaymentMethodMockBank, Status: repository.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("seed tx: %v", err)
	}
	return tx
}

// TestPaymentHandler_RefundAccess: refunds are for the order's seller or an admin
// only; the buyer, other users, service principals and anonymous callers are
// rejected and the payment stays PAID.
func TestPaymentHandler_RefundAccess(t *testing.T) {
	user := commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	svc := commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", context.Background(), codes.Unauthenticated},
		{"buyer", principalCtx("buyer-1", user), codes.PermissionDenied},
		{"other user", principalCtx("seller-2", user), codes.PermissionDenied},
		{"service principal", principalCtx("seller-1", svc), codes.PermissionDenied},
		{"order's seller", principalCtx("seller-1", user), codes.OK},
		{"admin", principalCtx("admin-1", user, "admin"), codes.OK},
	}
	for _, tc := range tests {
		for _, byOrderID := range []bool{false, true} {
			name := tc.name
			if byOrderID {
				name += "/by-order-id"
			}
			t.Run(name, func(t *testing.T) {
				h, repo, _, orders := setupHandlerTest()
				orders.orders["order-1"].SellerId = "seller-1"
				seeded := seedTx(t, repo)
				if _, err := repo.UpdateTransactionStatus(context.Background(), seeded.ID, repository.PaymentStatusPaid, "ref"); err != nil {
					t.Fatalf("mark paid: %v", err)
				}
				ref := seeded.ID
				if byOrderID {
					ref = "order-1"
				}
				_, err := h.RefundPayment(tc.ctx, &paymentv1.RefundPaymentRequest{PaymentId: ref, Amount: 1000, Reason: "r"})
				if got := status.Code(err); got != tc.want {
					t.Fatalf("code = %v, want %v (err=%v)", got, tc.want, err)
				}
				after, _ := repo.GetTransaction(context.Background(), seeded.ID)
				refunded := after.Status == repository.PaymentStatusRefunded
				if refunded != (tc.want == codes.OK) {
					t.Fatalf("refunded = %v for %s", refunded, tc.name)
				}
			})
		}
	}
}

func TestPaymentHandler_SellerWalletAndPayout(t *testing.T) {
	h, _, ledger, _ := setupHandlerTest()

	principal := &commonv1.Principal{
		Id:     "seller-1",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"payment:write", "payment:read"},
	}
	ctx := interceptor.ContextWithPrincipal(context.Background(), principal)

	t.Run("GetSellerWallet", func(t *testing.T) {
		res, err := h.GetSellerWallet(ctx, &paymentv1.GetSellerWalletRequest{
			SellerId: "seller-1",
		})
		if err != nil {
			t.Fatalf("GetSellerWallet failed: %v", err)
		}
		if res.Wallet.SellerId != "seller-1" {
			t.Errorf("want seller-1, got %s", res.Wallet.SellerId)
		}
		if res.Wallet.Balance != 0 {
			t.Errorf("want initial balance 0, got %d", res.Wallet.Balance)
		}
	})

	t.Run("RequestPayout and ListPayoutHistory", func(t *testing.T) {
		// Settlement credit on the ledger
		_, _ = ledger.AppendEntry(ctx, repository.LedgerEntry{
			SellerID: "seller-1", Type: repository.LedgerTypeOrderSettlement,
			Amount: 1000000, Status: repository.LedgerStatusCompleted,
		})

		// Request Payout
		payoutRes, err := h.RequestPayout(ctx, &paymentv1.RequestPayoutRequest{
			SellerId:      "seller-1",
			Amount:        300000,
			BankCode:      "VCB",
			AccountNumber: "1234567890",
			AccountName:   "NGUYEN VAN A",
		})
		if err != nil {
			t.Fatalf("RequestPayout failed: %v", err)
		}
		if payoutRes.Payout.Amount != 300000 {
			t.Errorf("want amount 300000, got %d", payoutRes.Payout.Amount)
		}
		if payoutRes.Payout.Status != paymentv1.PayoutStatus_PAYOUT_STATUS_PENDING {
			t.Errorf("want status PENDING, got %v", payoutRes.Payout.Status)
		}

		// List Payout History
		listRes, err := h.ListPayoutHistory(ctx, &paymentv1.ListPayoutHistoryRequest{
			SellerId: "seller-1",
		})
		if err != nil {
			t.Fatalf("ListPayoutHistory failed: %v", err)
		}
		if len(listRes.Payouts) != 1 {
			t.Fatalf("want 1 payout in history, got %d", len(listRes.Payouts))
		}
		if listRes.Payouts[0].Id != payoutRes.Payout.Id {
			t.Errorf("expected payout ID %s, got %s", payoutRes.Payout.Id, listRes.Payouts[0].Id)
		}
	})

	t.Run("RequestPayout insufficient balance", func(t *testing.T) {
		_, err := h.RequestPayout(ctx, &paymentv1.RequestPayoutRequest{
			SellerId:      "seller-1",
			Amount:        2000000, // exceeds balance
			BankCode:      "VCB",
			AccountNumber: "1234567890",
			AccountName:   "NGUYEN VAN A",
		})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})
}

func TestPaymentHandler_RefundPayment(t *testing.T) {
	h, paymentRepo, _, _ := setupHandlerTest()

	principal := &commonv1.Principal{
		Id:     "admin-1",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"admin"},
	}
	ctx := interceptor.ContextWithPrincipal(context.Background(), principal)

	// Create a PAID transaction
	tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
		ID:      "tx-refund-hdl",
		OrderID: "order-refund-1",
		Amount:  500000,
		Status:  repository.PaymentStatusPaid,
	})

	t.Run("success refund", func(t *testing.T) {
		res, err := h.RefundPayment(ctx, &paymentv1.RefundPaymentRequest{
			PaymentId: tx.ID,
			Amount:    500000,
			Reason:    "Customer return",
		})
		if err != nil {
			t.Fatalf("RefundPayment failed: %v", err)
		}
		if !res.Success {
			t.Error("expected success = true")
		}
		if res.Transaction.Status != paymentv1.PaymentStatus_PAYMENT_STATUS_REFUNDED {
			t.Errorf("want status REFUNDED, got %v", res.Transaction.Status)
		}
	})

	t.Run("refund not found", func(t *testing.T) {
		_, err := h.RefundPayment(ctx, &paymentv1.RefundPaymentRequest{
			PaymentId: "missing-tx",
			Amount:    100000,
			Reason:    "reason",
		})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("refund missing payment id", func(t *testing.T) {
		_, err := h.RefundPayment(ctx, &paymentv1.RefundPaymentRequest{
			PaymentId: "",
			Amount:    100000,
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})
}
