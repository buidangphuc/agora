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

func setupHandlerTest() (*handler.PaymentHandler, *repository.InMemoryPaymentRepository, *repository.InMemoryWalletRepository, *mockOrderClient) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	paymentRepo := repository.NewInMemoryPaymentRepository()
	walletRepo := repository.NewInMemoryWalletRepository()
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
	svc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger)
	h := handler.NewPaymentHandler(svc, logger)
	return h, paymentRepo, walletRepo, orderClient
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

func TestPaymentHandler_SellerWalletAndPayout(t *testing.T) {
	h, _, walletRepo, _ := setupHandlerTest()

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
		// Credit seller wallet
		_, _ = walletRepo.UpdateWalletBalance(ctx, "seller-1", 1000000)

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
		Scopes: []string{"payment:write", "payment:read"},
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
