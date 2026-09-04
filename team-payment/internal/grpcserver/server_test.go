package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/config"
	"github.com/buidangphuc/team-payment/internal/grpcserver"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

type mockOrderClient struct {
	orders map[string]*orderv1.Order
}

func (m *mockOrderClient) GetOrder(_ context.Context, req *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	o, ok := m.orders[req.GetId()]
	if !ok {
		return nil, service.ErrOrderNotFound
	}
	return &orderv1.GetOrderResponse{Order: o}, nil
}

func (m *mockOrderClient) UpdateOrderStatus(_ context.Context, req *orderv1.UpdateOrderStatusRequest, _ ...grpc.CallOption) (*orderv1.UpdateOrderStatusResponse, error) {
	o, ok := m.orders[req.GetId()]
	if !ok {
		return nil, service.ErrOrderNotFound
	}
	o.Status = req.GetStatus()
	return &orderv1.UpdateOrderStatusResponse{Order: o}, nil
}

func principalCtx(t *testing.T, userID string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs(
		"x-principal-id", userID,
		"x-principal-type", "user",
		"x-principal-scopes", "payment.write,payment.read",
	)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func setupTestServer(t *testing.T) (paymentv1.PaymentServiceClient, *mockOrderClient, repository.PaymentRepository, repository.WalletRepository) {
	t.Helper()

	mockOrder := &mockOrderClient{
		orders: map[string]*orderv1.Order{
			"order-123": {
				Id:          "order-123",
				BuyerId:     "user-1",
				TotalAmount: 250000,
				Currency:    "VND",
				Status:      orderv1.OrderStatus_ORDER_STATUS_PENDING,
			},
			"order-completed": {
				Id:          "order-completed",
				BuyerId:     "user-1",
				TotalAmount: 100000,
				Currency:    "VND",
				Status:      orderv1.OrderStatus_ORDER_STATUS_COMPLETED,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	paymentRepo := repository.NewInMemoryPaymentRepository()
	walletRepo := repository.NewInMemoryWalletRepository()
	paymentSvc := service.NewPaymentService(paymentRepo, walletRepo, mockOrder, logger)
	paymentHdl := handler.NewPaymentHandler(paymentSvc, logger)

	cfg := &config.Settings{
		Server: config.Server{Port: 0, ReflectionEnabled: true},
	}
	srv := grpcserver.Build(cfg, paymentHdl, nil, logger)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		srv.GracefulStop()
		_ = lis.Close()
	})

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return paymentv1.NewPaymentServiceClient(conn), mockOrder, paymentRepo, walletRepo
}

func TestPaymentFlow_Success(t *testing.T) {
	client, mockOrder, _, _ := setupTestServer(t)
	ctx, cancel := principalCtx(t, "user-1")
	defer cancel()

	// 1. Create Payment Transaction
	createResp, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		OrderId: "order-123",
		Method:  paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	tx := createResp.GetTransaction()
	if tx.GetOrderId() != "order-123" {
		t.Fatalf("want order_id order-123, got %s", tx.GetOrderId())
	}
	if tx.GetAmount() != 250000 {
		t.Fatalf("want amount 250000, got %d", tx.GetAmount())
	}
	if tx.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PENDING {
		t.Fatalf("want status PENDING, got %v", tx.GetStatus())
	}
	if createResp.GetPaymentUrl() != "/checkout/pay/order-123" {
		t.Fatalf("want payment url /checkout/pay/order-123, got %s", createResp.GetPaymentUrl())
	}

	// 2. Get Payment by OrderID
	getResp, err := client.GetPayment(ctx, &paymentv1.GetPaymentRequest{
		OrderId: "order-123",
	})
	if err != nil {
		t.Fatalf("GetPayment by order_id: %v", err)
	}
	if getResp.GetTransaction().GetId() != tx.GetId() {
		t.Fatalf("want tx id %s, got %s", tx.GetId(), getResp.GetTransaction().GetId())
	}

	// 3. Get Payment by Transaction ID
	getByIDResp, err := client.GetPayment(ctx, &paymentv1.GetPaymentRequest{
		Id: tx.GetId(),
	})
	if err != nil {
		t.Fatalf("GetPayment by id: %v", err)
	}
	if getByIDResp.GetTransaction().GetId() != tx.GetId() {
		t.Fatalf("want tx id %s, got %s", tx.GetId(), getByIDResp.GetTransaction().GetId())
	}

	// 4. Process Mock Payment (Simulate Success)
	procResp, err := client.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{
		TransactionId:   tx.GetId(),
		SimulateSuccess: true,
	})
	if err != nil {
		t.Fatalf("ProcessMockPayment: %v", err)
	}
	if !procResp.GetSuccess() {
		t.Fatalf("expected success true, got false")
	}
	if procResp.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
		t.Fatalf("want status PAID, got %v", procResp.GetTransaction().GetStatus())
	}

	// AD4 (SA-H3): settle is now event-carried via the transactional outbox, not
	// a synchronous order.UpdateOrderStatus RPC. The order transition is driven by
	// the emitted PaymentSettled event (consumed by team-order), so from
	// team-payment's side the order stays PENDING here.
	if mockOrder.orders["order-123"].Status != orderv1.OrderStatus_ORDER_STATUS_PENDING {
		t.Fatalf("want order to remain PENDING (no sync call), got %v", mockOrder.orders["order-123"].Status)
	}

	// 5. Refund Payment
	refundResp, err := client.RefundPayment(ctx, &paymentv1.RefundPaymentRequest{
		PaymentId: tx.GetId(),
		Amount:    250000,
		Reason:    "Product returned",
	})
	if err != nil {
		t.Fatalf("RefundPayment: %v", err)
	}
	if !refundResp.GetSuccess() {
		t.Fatalf("expected refund success true")
	}
	if refundResp.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_REFUNDED {
		t.Fatalf("want status REFUNDED, got %v", refundResp.GetTransaction().GetStatus())
	}
}

func TestPaymentFlow_SimulateFailure(t *testing.T) {
	client, mockOrder, _, _ := setupTestServer(t)
	ctx, cancel := principalCtx(t, "user-1")
	defer cancel()

	// 1. Create Payment Transaction
	createResp, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		OrderId: "order-123",
		Method:  paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_BANK,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	tx := createResp.GetTransaction()

	// 2. Process Mock Payment (Simulate Failure)
	procResp, err := client.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{
		TransactionId:   tx.GetId(),
		SimulateSuccess: false,
	})
	if err != nil {
		t.Fatalf("ProcessMockPayment: %v", err)
	}
	if procResp.GetSuccess() {
		t.Fatalf("expected success false, got true")
	}
	if procResp.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED {
		t.Fatalf("want status FAILED, got %v", procResp.GetTransaction().GetStatus())
	}

	// Verify order status is still PENDING
	if mockOrder.orders["order-123"].Status != orderv1.OrderStatus_ORDER_STATUS_PENDING {
		t.Fatalf("want order status PENDING, got %v", mockOrder.orders["order-123"].Status)
	}
}

func TestSellerWallet_FullFlow(t *testing.T) {
	client, _, _, walletRepo := setupTestServer(t)
	ctx, cancel := principalCtx(t, "seller-99")
	defer cancel()

	// 1. Get initial seller wallet
	walletResp, err := client.GetSellerWallet(ctx, &paymentv1.GetSellerWalletRequest{
		SellerId: "seller-99",
	})
	if err != nil {
		t.Fatalf("GetSellerWallet: %v", err)
	}
	if walletResp.GetWallet().GetBalance() != 0 {
		t.Fatalf("want balance 0, got %d", walletResp.GetWallet().GetBalance())
	}
	if walletResp.GetWallet().GetSellerId() != "seller-99" {
		t.Fatalf("want seller_id seller-99, got %s", walletResp.GetWallet().GetSellerId())
	}

	// 2. Simulate order settlement: credit seller wallet directly in repo
	_, err = walletRepo.UpdateWalletBalance(ctx, "seller-99", 1500000)
	if err != nil {
		t.Fatalf("UpdateWalletBalance: %v", err)
	}

	// 3. Verify balance updated
	walletResp2, err := client.GetSellerWallet(ctx, &paymentv1.GetSellerWalletRequest{
		SellerId: "seller-99",
	})
	if err != nil {
		t.Fatalf("GetSellerWallet 2: %v", err)
	}
	if walletResp2.GetWallet().GetBalance() != 1500000 {
		t.Fatalf("want balance 1500000, got %d", walletResp2.GetWallet().GetBalance())
	}

	// 4. Request Payout
	payoutResp, err := client.RequestPayout(ctx, &paymentv1.RequestPayoutRequest{
		SellerId:      "seller-99",
		Amount:        500000,
		BankCode:      "VCB",
		AccountNumber: "0123456789",
		AccountName:   "NGUYEN VAN C",
	})
	if err != nil {
		t.Fatalf("RequestPayout: %v", err)
	}
	payout := payoutResp.GetPayout()
	if payout.GetAmount() != 500000 {
		t.Fatalf("want amount 500000, got %d", payout.GetAmount())
	}
	if payout.GetStatus() != paymentv1.PayoutStatus_PAYOUT_STATUS_PENDING {
		t.Fatalf("want status PENDING, got %v", payout.GetStatus())
	}

	// 5. Verify remaining balance (1500000 - 500000 = 1000000)
	walletResp3, err := client.GetSellerWallet(ctx, &paymentv1.GetSellerWalletRequest{
		SellerId: "seller-99",
	})
	if err != nil {
		t.Fatalf("GetSellerWallet 3: %v", err)
	}
	if walletResp3.GetWallet().GetBalance() != 1000000 {
		t.Fatalf("want balance 1000000, got %d", walletResp3.GetWallet().GetBalance())
	}

	// 6. List Payout History
	historyResp, err := client.ListPayoutHistory(ctx, &paymentv1.ListPayoutHistoryRequest{
		SellerId: "seller-99",
	})
	if err != nil {
		t.Fatalf("ListPayoutHistory: %v", err)
	}
	if len(historyResp.GetPayouts()) != 1 {
		t.Fatalf("want 1 payout in history, got %d", len(historyResp.GetPayouts()))
	}
	if historyResp.GetPayouts()[0].GetId() != payout.GetId() {
		t.Fatalf("want payout ID %s, got %s", payout.GetId(), historyResp.GetPayouts()[0].GetId())
	}

	// 7. Request Payout exceeding balance -> should fail with FailedPrecondition
	_, err = client.RequestPayout(ctx, &paymentv1.RequestPayoutRequest{
		SellerId:      "seller-99",
		Amount:        2000000,
		BankCode:      "VCB",
		AccountNumber: "0123456789",
		AccountName:   "NGUYEN VAN C",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}

func TestCreatePayment_Errors(t *testing.T) {
	client, _, _, _ := setupTestServer(t)

	t.Run("missing principal", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{OrderId: "order-123"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", err)
		}
	})

	t.Run("empty order id", func(t *testing.T) {
		ctx, cancel := principalCtx(t, "user-1")
		defer cancel()

		_, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{OrderId: ""})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("order not found", func(t *testing.T) {
		ctx, cancel := principalCtx(t, "user-1")
		defer cancel()

		_, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{OrderId: "order-999"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("order not pending", func(t *testing.T) {
		ctx, cancel := principalCtx(t, "user-1")
		defer cancel()

		_, err := client.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{OrderId: "order-completed"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})
}

func TestGetPayment_Errors(t *testing.T) {
	client, _, _, _ := setupTestServer(t)
	ctx, cancel := principalCtx(t, "user-1")
	defer cancel()

	t.Run("missing both params", func(t *testing.T) {
		_, err := client.GetPayment(ctx, &paymentv1.GetPaymentRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("not found by id", func(t *testing.T) {
		_, err := client.GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: "non-existent"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("not found by order_id", func(t *testing.T) {
		_, err := client.GetPayment(ctx, &paymentv1.GetPaymentRequest{OrderId: "non-existent-order"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})
}

func TestProcessMockPayment_Errors(t *testing.T) {
	client, _, _, _ := setupTestServer(t)
	ctx, cancel := principalCtx(t, "user-1")
	defer cancel()

	t.Run("missing transaction id", func(t *testing.T) {
		_, err := client.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("not found transaction id", func(t *testing.T) {
		_, err := client.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{
			TransactionId:   "non-existent-tx",
			SimulateSuccess: true,
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})
}
