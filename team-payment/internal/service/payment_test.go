package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

type mockOrderClient struct {
	orders      map[string]*orderv1.Order
	err         error
	updateCalls int // counts UpdateOrderStatus calls (must stay 0 after AD4)
}

func (m *mockOrderClient) GetOrder(_ context.Context, req *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	o, ok := m.orders[req.GetId()]
	if !ok {
		return nil, errors.New("order not found")
	}
	return &orderv1.GetOrderResponse{Order: o}, nil
}

func (m *mockOrderClient) UpdateOrderStatus(_ context.Context, req *orderv1.UpdateOrderStatusRequest, _ ...grpc.CallOption) (*orderv1.UpdateOrderStatusResponse, error) {
	m.updateCalls++
	if m.err != nil {
		return nil, m.err
	}
	o, ok := m.orders[req.GetId()]
	if !ok {
		return nil, errors.New("order not found")
	}
	o.Status = req.GetStatus()
	return &orderv1.UpdateOrderStatusResponse{Order: o}, nil
}

func setupService() (*service.PaymentService, *repository.InMemoryPaymentRepository, *repository.InMemoryWalletRepository, *mockOrderClient) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	paymentRepo := repository.NewInMemoryPaymentRepository()
	walletRepo := repository.NewInMemoryWalletRepository()
	orderClient := &mockOrderClient{
		orders: make(map[string]*orderv1.Order),
	}
	svc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger)
	return svc, paymentRepo, walletRepo, orderClient
}

// ── Payment Processing Tests ─────────────────────────────────────────

func TestService_CreatePayment(t *testing.T) {
	ctx := context.Background()

	t.Run("success create new payment", func(t *testing.T) {
		svc, _, _, orderClient := setupService()
		orderClient.orders["order-1"] = &orderv1.Order{
			Id:          "order-1",
			BuyerId:     "buyer-1",
			TotalAmount: 200000,
			Currency:    "VND",
			Status:      orderv1.OrderStatus_ORDER_STATUS_PENDING,
		}

		tx, url, err := svc.CreatePayment(ctx, "order-1", "buyer-1", repository.PaymentMethodMockMoMo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.ID == "" {
			t.Fatal("expected generated tx ID")
		}
		if tx.OrderID != "order-1" || tx.Amount != 200000 {
			t.Fatalf("mismatched transaction data: %+v", tx)
		}
		if tx.Status != repository.PaymentStatusPending {
			t.Fatalf("expected PENDING status, got %v", tx.Status)
		}
		if url != "/checkout/pay/order-1" {
			t.Fatalf("expected payment URL /checkout/pay/order-1, got %s", url)
		}
	})

	t.Run("empty order id", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, err := svc.CreatePayment(ctx, "", "buyer-1", repository.PaymentMethodMockMoMo)
		if err == nil {
			t.Fatal("expected error for empty order ID")
		}
	})

	t.Run("order not found", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, err := svc.CreatePayment(ctx, "missing-order", "buyer-1", repository.PaymentMethodMockMoMo)
		if !errors.Is(err, service.ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}
	})

	t.Run("order not in pending state", func(t *testing.T) {
		svc, _, _, orderClient := setupService()
		orderClient.orders["order-paid"] = &orderv1.Order{
			Id:     "order-paid",
			Status: orderv1.OrderStatus_ORDER_STATUS_PAID,
		}

		_, _, err := svc.CreatePayment(ctx, "order-paid", "buyer-1", repository.PaymentMethodMockMoMo)
		if !errors.Is(err, service.ErrInvalidOrderState) {
			t.Fatalf("expected ErrInvalidOrderState, got %v", err)
		}
	})

	t.Run("idempotent return existing transaction", func(t *testing.T) {
		svc, _, _, orderClient := setupService()
		orderClient.orders["order-1"] = &orderv1.Order{
			Id:          "order-1",
			BuyerId:     "buyer-1",
			TotalAmount: 200000,
			Currency:    "VND",
			Status:      orderv1.OrderStatus_ORDER_STATUS_PENDING,
		}

		tx1, _, err := svc.CreatePayment(ctx, "order-1", "buyer-1", repository.PaymentMethodMockMoMo)
		if err != nil {
			t.Fatalf("first CreatePayment failed: %v", err)
		}

		tx2, _, err := svc.CreatePayment(ctx, "order-1", "buyer-1", repository.PaymentMethodMockMoMo)
		if err != nil {
			t.Fatalf("second CreatePayment failed: %v", err)
		}
		if tx1.ID != tx2.ID {
			t.Fatalf("expected same transaction ID %s, got %s", tx1.ID, tx2.ID)
		}
	})
}

func TestService_GetPayment(t *testing.T) {
	ctx := context.Background()
	svc, paymentRepo, _, _ := setupService()

	tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
		ID:      "tx-100",
		OrderID: "order-100",
		Amount:  100000,
	})

	t.Run("get by id", func(t *testing.T) {
		res, err := svc.GetPayment(ctx, tx.ID, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ID != tx.ID {
			t.Fatalf("expected tx ID %s, got %s", tx.ID, res.ID)
		}
	})

	t.Run("get by order id", func(t *testing.T) {
		res, err := svc.GetPayment(ctx, "", tx.OrderID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ID != tx.ID {
			t.Fatalf("expected tx ID %s, got %s", tx.ID, res.ID)
		}
	})

	t.Run("missing params", func(t *testing.T) {
		_, err := svc.GetPayment(ctx, "", "")
		if err == nil {
			t.Fatal("expected error for empty id and order_id")
		}
	})
}

func TestService_ProcessMockPayment(t *testing.T) {
	ctx := context.Background()

	t.Run("success simulation", func(t *testing.T) {
		svc, paymentRepo, _, orderClient := setupService()
		orderClient.orders["order-1"] = &orderv1.Order{
			Id:     "order-1",
			Status: orderv1.OrderStatus_ORDER_STATUS_PENDING,
		}

		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			OrderID: "order-1",
			Amount:  300000,
			Status:  repository.PaymentStatusPending,
		})

		updatedTx, success, _, err := svc.ProcessMockPayment(ctx, tx.ID, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !success {
			t.Fatal("expected success = true")
		}
		if updatedTx.Status != repository.PaymentStatusPaid {
			t.Fatalf("expected PAID status, got %v", updatedTx.Status)
		}
		// AD4 (SA-H3): settle no longer calls order.UpdateOrderStatus synchronously.
		// The order transition is driven by the emitted PaymentSettled event
		// (asserted in settle_outbox_test.go), so the order stays PENDING here.
		if orderClient.orders["order-1"].Status != orderv1.OrderStatus_ORDER_STATUS_PENDING {
			t.Fatalf("expected order to remain PENDING (no sync call), got %v", orderClient.orders["order-1"].Status)
		}
	})

	t.Run("failure simulation", func(t *testing.T) {
		svc, paymentRepo, _, orderClient := setupService()
		orderClient.orders["order-2"] = &orderv1.Order{
			Id:     "order-2",
			Status: orderv1.OrderStatus_ORDER_STATUS_PENDING,
		}

		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			OrderID: "order-2",
			Amount:  300000,
			Status:  repository.PaymentStatusPending,
		})

		updatedTx, success, _, err := svc.ProcessMockPayment(ctx, tx.ID, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if success {
			t.Fatal("expected success = false")
		}
		if updatedTx.Status != repository.PaymentStatusFailed {
			t.Fatalf("expected FAILED status, got %v", updatedTx.Status)
		}
	})

	t.Run("already paid idempotent", func(t *testing.T) {
		svc, paymentRepo, _, _ := setupService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			OrderID: "order-3",
			Amount:  300000,
			Status:  repository.PaymentStatusPaid,
		})

		updatedTx, success, _, err := svc.ProcessMockPayment(ctx, tx.ID, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !success {
			t.Fatal("expected success = true")
		}
		if updatedTx.Status != repository.PaymentStatusPaid {
			t.Fatalf("expected PAID status, got %v", updatedTx.Status)
		}
	})

	t.Run("transaction not found", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, _, err := svc.ProcessMockPayment(ctx, "non-existent", true)
		if !errors.Is(err, service.ErrTransactionNotFound) {
			t.Fatalf("expected ErrTransactionNotFound, got %v", err)
		}
	})
}

// ── Refund Tests ─────────────────────────────────────────────────────

func TestService_RefundPayment(t *testing.T) {
	ctx := context.Background()

	t.Run("success refund paid transaction", func(t *testing.T) {
		svc, paymentRepo, _, _ := setupService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			ID:      "tx-refund-1",
			OrderID: "order-10",
			Amount:  500000,
			Status:  repository.PaymentStatusPaid,
		})

		updated, ok, msg, err := svc.RefundPayment(ctx, tx.ID, 500000, "Customer cancellation")
		if err != nil {
			t.Fatalf("RefundPayment failed: %v", err)
		}
		if !ok {
			t.Fatal("expected ok = true")
		}
		if updated.Status != repository.PaymentStatusRefunded {
			t.Errorf("want status REFUNDED, got %v", updated.Status)
		}
		if msg == "" {
			t.Error("expected non-empty message")
		}
	})

	t.Run("missing payment id", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, _, err := svc.RefundPayment(ctx, "", 100000, "reason")
		if err == nil {
			t.Fatal("expected error for empty payment id")
		}
	})

	t.Run("invalid refund amount", func(t *testing.T) {
		svc, paymentRepo, _, _ := setupService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			ID:      "tx-refund-2",
			OrderID: "order-20",
			Amount:  200000,
			Status:  repository.PaymentStatusPaid,
		})

		// Amount <= 0
		_, _, _, err := svc.RefundPayment(ctx, tx.ID, 0, "reason")
		if !errors.Is(err, service.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}

		// Amount > tx.Amount
		_, _, _, err = svc.RefundPayment(ctx, tx.ID, 300000, "reason")
		if err == nil {
			t.Fatal("expected error when refund amount exceeds tx amount")
		}
	})

	t.Run("refund unpaid transaction error", func(t *testing.T) {
		svc, paymentRepo, _, _ := setupService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			ID:      "tx-refund-3",
			OrderID: "order-30",
			Amount:  200000,
			Status:  repository.PaymentStatusPending,
		})

		_, _, _, err := svc.RefundPayment(ctx, tx.ID, 200000, "reason")
		if !errors.Is(err, service.ErrInvalidRefund) {
			t.Errorf("expected ErrInvalidRefund for pending tx, got %v", err)
		}
	})

	t.Run("refund already refunded transaction error", func(t *testing.T) {
		svc, paymentRepo, _, _ := setupService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			ID:      "tx-refund-4",
			OrderID: "order-40",
			Amount:  200000,
			Status:  repository.PaymentStatusRefunded,
		})

		_, _, _, err := svc.RefundPayment(ctx, tx.ID, 200000, "reason")
		if !errors.Is(err, service.ErrInvalidRefund) {
			t.Errorf("expected ErrInvalidRefund for refunded tx, got %v", err)
		}
	})

	t.Run("refund transaction not found", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, _, err := svc.RefundPayment(ctx, "non-existent", 100000, "reason")
		if !errors.Is(err, service.ErrTransactionNotFound) {
			t.Errorf("expected ErrTransactionNotFound, got %v", err)
		}
	})
}

// ── Seller Wallet & Payout Tests ─────────────────────────────────────

func TestService_GetSellerWallet(t *testing.T) {
	ctx := context.Background()

	t.Run("first access creates new wallet", func(t *testing.T) {
		svc, _, _, _ := setupService()
		w, err := svc.GetSellerWallet(ctx, "seller-new")
		if err != nil {
			t.Fatalf("GetSellerWallet failed: %v", err)
		}
		if w.SellerID != "seller-new" {
			t.Errorf("want seller-new, got %s", w.SellerID)
		}
		if w.Balance != 0 {
			t.Errorf("want balance 0, got %d", w.Balance)
		}
	})

	t.Run("existing wallet returned with correct balance", func(t *testing.T) {
		svc, _, walletRepo, _ := setupService()
		_, _ = walletRepo.UpdateWalletBalance(ctx, "seller-existing", 1500000)

		w, err := svc.GetSellerWallet(ctx, "seller-existing")
		if err != nil {
			t.Fatalf("GetSellerWallet failed: %v", err)
		}
		if w.Balance != 1500000 {
			t.Errorf("want balance 1500000, got %d", w.Balance)
		}
	})

	t.Run("empty seller id error", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.GetSellerWallet(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}

func TestService_RequestPayout(t *testing.T) {
	ctx := context.Background()

	t.Run("success payout request deducts balance", func(t *testing.T) {
		svc, _, walletRepo, _ := setupService()
		// Credit seller wallet with 1,000,000 VND
		_, _ = walletRepo.UpdateWalletBalance(ctx, "seller-payout-1", 1000000)

		payout, err := svc.RequestPayout(ctx, "seller-payout-1", 400000, "VCB", "1234567890", "NGUYEN VAN A")
		if err != nil {
			t.Fatalf("RequestPayout failed: %v", err)
		}
		if payout.ID == "" {
			t.Fatal("expected non-empty payout ID")
		}
		if payout.Amount != 400000 {
			t.Errorf("want amount 400000, got %d", payout.Amount)
		}
		if payout.Status != repository.PayoutStatusPending {
			t.Errorf("want PENDING status, got %v", payout.Status)
		}

		// Verify wallet balance is deducted (1,000,000 - 400,000 = 600,000)
		wallet, err := walletRepo.GetWalletBySellerID(ctx, "seller-payout-1")
		if err != nil {
			t.Fatalf("GetWalletBySellerID failed: %v", err)
		}
		if wallet.Balance != 600000 {
			t.Errorf("want balance 600000, got %d", wallet.Balance)
		}

		// Verify wallet transaction created
		txs, err := walletRepo.ListWalletTransactions(ctx, wallet.ID)
		if err != nil {
			t.Fatalf("ListWalletTransactions failed: %v", err)
		}
		if len(txs) != 1 {
			t.Fatalf("want 1 transaction, got %d", len(txs))
		}
		if txs[0].Amount != -400000 || txs[0].Type != repository.WalletTxTypePayout {
			t.Errorf("mismatched wallet transaction: %+v", txs[0])
		}
	})

	t.Run("insufficient wallet balance error", func(t *testing.T) {
		svc, _, walletRepo, _ := setupService()
		_, _ = walletRepo.UpdateWalletBalance(ctx, "seller-payout-2", 100000)

		_, err := svc.RequestPayout(ctx, "seller-payout-2", 500000, "VCB", "1234567890", "NGUYEN VAN A")
		if !errors.Is(err, repository.ErrInsufficientBalance) {
			t.Fatalf("expected ErrInsufficientBalance, got %v", err)
		}

		// Verify balance did not change
		wallet, _ := walletRepo.GetWalletBySellerID(ctx, "seller-payout-2")
		if wallet.Balance != 100000 {
			t.Errorf("expected balance to remain 100000, got %d", wallet.Balance)
		}
	})

	t.Run("invalid amount error", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.RequestPayout(ctx, "seller-payout-3", 0, "VCB", "1234567890", "NGUYEN VAN A")
		if !errors.Is(err, service.ErrInvalidAmount) {
			t.Fatalf("expected ErrInvalidAmount for 0 amount, got %v", err)
		}

		_, err = svc.RequestPayout(ctx, "seller-payout-3", -50000, "VCB", "1234567890", "NGUYEN VAN A")
		if !errors.Is(err, service.ErrInvalidAmount) {
			t.Fatalf("expected ErrInvalidAmount for negative amount, got %v", err)
		}
	})

	t.Run("missing bank info error", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.RequestPayout(ctx, "seller-payout-4", 100000, "", "1234567890", "NGUYEN VAN A")
		if err == nil {
			t.Fatal("expected error for empty bank code")
		}

		_, err = svc.RequestPayout(ctx, "seller-payout-4", 100000, "VCB", "", "NGUYEN VAN A")
		if err == nil {
			t.Fatal("expected error for empty account number")
		}

		_, err = svc.RequestPayout(ctx, "seller-payout-4", 100000, "VCB", "1234567890", "")
		if err == nil {
			t.Fatal("expected error for empty account name")
		}
	})

	t.Run("missing seller id error", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.RequestPayout(ctx, "", 100000, "VCB", "1234567890", "NGUYEN VAN A")
		if err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}

func TestService_ListPayoutHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("list multiple payouts", func(t *testing.T) {
		svc, _, walletRepo, _ := setupService()
		_, _ = walletRepo.UpdateWalletBalance(ctx, "seller-hist", 2000000)

		_, err := svc.RequestPayout(ctx, "seller-hist", 300000, "VCB", "111", "ACC 1")
		if err != nil {
			t.Fatalf("payout 1 failed: %v", err)
		}
		_, err = svc.RequestPayout(ctx, "seller-hist", 500000, "TCB", "222", "ACC 2")
		if err != nil {
			t.Fatalf("payout 2 failed: %v", err)
		}

		history, err := svc.ListPayoutHistory(ctx, "seller-hist")
		if err != nil {
			t.Fatalf("ListPayoutHistory failed: %v", err)
		}
		if len(history) != 2 {
			t.Fatalf("want 2 payouts, got %d", len(history))
		}
	})

	t.Run("empty history for new seller", func(t *testing.T) {
		svc, _, _, _ := setupService()
		history, err := svc.ListPayoutHistory(ctx, "seller-no-history")
		if err != nil {
			t.Fatalf("ListPayoutHistory failed: %v", err)
		}
		if len(history) != 0 {
			t.Errorf("want 0 payouts, got %d", len(history))
		}
	})

	t.Run("empty seller id error", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, err := svc.ListPayoutHistory(ctx, "")
		if err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}
