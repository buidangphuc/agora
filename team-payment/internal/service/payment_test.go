package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
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
	svc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger,
		service.WithSettlementLedger(repository.NewInMemorySettlementLedger(paymentRepo, repository.NewInMemoryLedgerRepository())))
	return svc, paymentRepo, walletRepo, orderClient
}

// setupWalletService wires a service with BOTH the payout-request store and the wallet
// ledger (the single source of truth for seller money).
func setupWalletService() (*service.PaymentService, *repository.InMemoryWalletRepository, *repository.InMemoryLedgerRepository) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	walletRepo := repository.NewInMemoryWalletRepository()
	ledger := repository.NewInMemoryLedgerRepository()
	svc := service.NewPaymentService(repository.NewInMemoryPaymentRepository(), walletRepo, &mockOrderClient{orders: map[string]*orderv1.Order{}}, logger, service.WithLedgerRepo(ledger))
	return svc, walletRepo, ledger
}

func credit(t *testing.T, ledger *repository.InMemoryLedgerRepository, seller string, amount int64) {
	t.Helper()
	if _, err := ledger.AppendEntry(context.Background(), repository.LedgerEntry{
		SellerID: seller, Type: repository.LedgerTypeOrderSettlement, Amount: amount, Status: repository.LedgerStatusCompleted,
	}); err != nil {
		t.Fatalf("credit: %v", err)
	}
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

	t.Run("caller is not the order's buyer", func(t *testing.T) {
		svc, repo, _, orderClient := setupService()
		orderClient.orders["order-1"] = &orderv1.Order{
			Id:          "order-1",
			BuyerId:     "buyer-1",
			TotalAmount: 200000,
			Currency:    "VND",
			Status:      orderv1.OrderStatus_ORDER_STATUS_PENDING,
		}

		_, _, err := svc.CreatePayment(ctx, "order-1", "intruder", repository.PaymentMethodMockMoMo)
		if !errors.Is(err, service.ErrNotOrderBuyer) {
			t.Fatalf("expected ErrNotOrderBuyer, got %v", err)
		}
		if _, err := repo.GetTransactionByOrderID(ctx, "order-1"); err == nil {
			t.Fatal("a transaction was created for a non-buyer")
		}

		// The real buyer's transaction is recorded against the order's buyer.
		tx, _, err := svc.CreatePayment(ctx, "order-1", "buyer-1", repository.PaymentMethodMockMoMo)
		if err != nil {
			t.Fatalf("buyer create: %v", err)
		}
		if tx.BuyerID != "buyer-1" {
			t.Fatalf("tx buyer = %q, want buyer-1", tx.BuyerID)
		}
		// An existing transaction is not handed to someone else either.
		if _, _, err := svc.CreatePayment(ctx, "order-1", "intruder", repository.PaymentMethodMockMoMo); !errors.Is(err, service.ErrNotOrderBuyer) {
			t.Fatalf("existing-tx path: expected ErrNotOrderBuyer, got %v", err)
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
			Id:      "order-paid",
			BuyerId: "buyer-1",
			Status:  orderv1.OrderStatus_ORDER_STATUS_PAID,
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

		updated, ok, msg, err := svc.RefundPayment(ctx, tx.ID, "r1", 500000, "Customer cancellation")
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
		_, _, _, err := svc.RefundPayment(ctx, "", "r2", 100000, "reason")
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
		_, _, _, err := svc.RefundPayment(ctx, tx.ID, "r3", 0, "reason")
		if !errors.Is(err, service.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got %v", err)
		}

		// Amount > tx.Amount
		_, _, _, err = svc.RefundPayment(ctx, tx.ID, "r4", 300000, "reason")
		if !errors.Is(err, service.ErrExceedsRemainder) {
			t.Fatalf("expected ErrExceedsRemainder, got %v", err)
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

		_, _, _, err := svc.RefundPayment(ctx, tx.ID, "r5", 200000, "reason")
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

		_, _, _, err := svc.RefundPayment(ctx, tx.ID, "r6", 200000, "reason")
		if !errors.Is(err, service.ErrInvalidRefund) {
			t.Errorf("expected ErrInvalidRefund for refunded tx, got %v", err)
		}
	})

	t.Run("refund transaction not found", func(t *testing.T) {
		svc, _, _, _ := setupService()
		_, _, _, err := svc.RefundPayment(ctx, "non-existent", "r7", 100000, "reason")
		if !errors.Is(err, service.ErrTransactionNotFound) {
			t.Errorf("expected ErrTransactionNotFound, got %v", err)
		}
	})
}

// ── Seller Wallet & Payout Tests ─────────────────────────────────────

func TestService_GetSellerWallet(t *testing.T) {
	ctx := context.Background()

	t.Run("no ledger entries means zero balance in VND", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		w, err := svc.GetSellerWallet(ctx, "seller-new")
		if err != nil {
			t.Fatalf("GetSellerWallet failed: %v", err)
		}
		if w.SellerID != "seller-new" || w.Balance != 0 || w.Currency != "VND" {
			t.Errorf("unexpected wallet: %+v", w)
		}
	})

	t.Run("balance equals the ledger sum", func(t *testing.T) {
		svc, _, ledger := setupWalletService()
		credit(t, ledger, "seller-existing", 1500000)
		credit(t, ledger, "seller-existing", 250000)
		credit(t, ledger, "other-seller", 999)
		if _, err := svc.RequestWalletPayout(ctx, "seller-existing", 100000); err != nil {
			t.Fatalf("payout: %v", err)
		}

		w, err := svc.GetSellerWallet(ctx, "seller-existing")
		if err != nil {
			t.Fatalf("GetSellerWallet failed: %v", err)
		}
		sum, _ := ledger.Balance(ctx, "seller-existing")
		if sum != 1650000 || w.Balance != sum {
			t.Errorf("want wallet balance == ledger sum 1650000, got wallet %d ledger %d", w.Balance, sum)
		}
	})

	t.Run("empty seller id error", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		if _, err := svc.GetSellerWallet(ctx, ""); err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}

func TestService_RequestPayout(t *testing.T) {
	ctx := context.Background()

	t.Run("payout debits the ledger and links the bank details to the entry", func(t *testing.T) {
		svc, walletRepo, ledger := setupWalletService()
		credit(t, ledger, "seller-payout-1", 1000000)

		payout, err := svc.RequestPayout(ctx, "seller-payout-1", 400000, "VCB", "1234567890", "NGUYEN VAN A")
		if err != nil {
			t.Fatalf("RequestPayout failed: %v", err)
		}
		if payout.ID == "" || payout.Amount != 400000 || payout.Status != repository.PayoutStatusPending {
			t.Errorf("unexpected payout: %+v", payout)
		}
		if bal, _ := ledger.Balance(ctx, "seller-payout-1"); bal != 600000 {
			t.Errorf("want ledger balance 600000, got %d", bal)
		}
		entries, _, _ := ledger.ListEntries(ctx, "seller-payout-1", 0, 10)
		var debit *repository.LedgerEntry
		for i := range entries {
			if entries[i].Amount == -400000 {
				debit = &entries[i]
			}
		}
		if debit == nil || debit.Type != repository.LedgerTypePayout || debit.Status != repository.LedgerStatusPending {
			t.Fatalf("missing PENDING payout debit: %+v", entries)
		}
		if payout.LedgerEntryID != debit.ID {
			t.Errorf("payout.LedgerEntryID = %q, want %q", payout.LedgerEntryID, debit.ID)
		}
		stored, err := walletRepo.GetPayoutRequest(ctx, payout.ID)
		if err != nil || stored.LedgerEntryID != debit.ID || stored.BankCode != "VCB" {
			t.Errorf("stored payout not linked: %+v err=%v", stored, err)
		}
	})

	t.Run("payout is limited by the ledger balance", func(t *testing.T) {
		svc, walletRepo, ledger := setupWalletService()
		credit(t, ledger, "seller-payout-2", 100000)

		_, err := svc.RequestPayout(ctx, "seller-payout-2", 500000, "VCB", "1234567890", "NGUYEN VAN A")
		if !errors.Is(err, repository.ErrInsufficientBalance) {
			t.Fatalf("expected ErrInsufficientBalance, got %v", err)
		}
		if bal, _ := ledger.Balance(ctx, "seller-payout-2"); bal != 100000 {
			t.Errorf("expected ledger balance to remain 100000, got %d", bal)
		}
		if list, _ := walletRepo.ListPayoutRequestsBySellerID(ctx, "seller-payout-2"); len(list) != 0 {
			t.Errorf("no payout request may be recorded, got %d", len(list))
		}
	})

	t.Run("seller who never earned anything cannot pay out", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		_, err := svc.RequestPayout(ctx, "seller-broke", 1, "VCB", "1234567890", "NGUYEN VAN A")
		if !errors.Is(err, repository.ErrInsufficientBalance) {
			t.Fatalf("expected ErrInsufficientBalance, got %v", err)
		}
	})

	t.Run("concurrent RequestPayout and RequestWalletPayout cannot overdraw", func(t *testing.T) {
		svc, _, ledger := setupWalletService()
		credit(t, ledger, "seller-race", 1000)

		const workers = 40
		var wg sync.WaitGroup
		var ok int32
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				var err error
				if i%2 == 0 {
					_, err = svc.RequestPayout(ctx, "seller-race", 100, "VCB", "123", "ACC")
				} else {
					_, err = svc.RequestWalletPayout(ctx, "seller-race", 100)
				}
				if err == nil {
					atomic.AddInt32(&ok, 1)
				} else if !errors.Is(err, repository.ErrInsufficientBalance) {
					t.Errorf("unexpected error: %v", err)
				}
			}(i)
		}
		wg.Wait()
		bal, _ := ledger.Balance(ctx, "seller-race")
		if ok != 10 || bal != 0 {
			t.Fatalf("want exactly 10 successful payouts and balance 0, got %d successes, balance %d", ok, bal)
		}
	})

	t.Run("invalid amount error", func(t *testing.T) {
		svc, _, _ := setupWalletService()
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
		svc, _, ledger := setupWalletService()
		credit(t, ledger, "seller-payout-4", 1000000)
		for _, c := range [][3]string{{"", "1", "A"}, {"VCB", "", "A"}, {"VCB", "1", ""}} {
			if _, err := svc.RequestPayout(ctx, "seller-payout-4", 100000, c[0], c[1], c[2]); err == nil {
				t.Fatalf("expected error for %v", c)
			}
		}
		if bal, _ := ledger.Balance(ctx, "seller-payout-4"); bal != 1000000 {
			t.Errorf("rejected requests must not debit, balance %d", bal)
		}
	})

	t.Run("missing seller id error", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		if _, err := svc.RequestPayout(ctx, "", 100000, "VCB", "1234567890", "NGUYEN VAN A"); err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}

func TestService_ListPayoutHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("list multiple payouts", func(t *testing.T) {
		svc, _, ledger := setupWalletService()
		credit(t, ledger, "seller-hist", 2000000)

		if _, err := svc.RequestPayout(ctx, "seller-hist", 300000, "VCB", "111", "ACC 1"); err != nil {
			t.Fatalf("payout 1 failed: %v", err)
		}
		if _, err := svc.RequestPayout(ctx, "seller-hist", 500000, "TCB", "222", "ACC 2"); err != nil {
			t.Fatalf("payout 2 failed: %v", err)
		}
		history, err := svc.ListPayoutHistory(ctx, "seller-hist")
		if err != nil || len(history) != 2 {
			t.Fatalf("want 2 payouts, got %d err=%v", len(history), err)
		}
	})

	t.Run("empty history for new seller", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		history, err := svc.ListPayoutHistory(ctx, "seller-no-history")
		if err != nil || len(history) != 0 {
			t.Errorf("want 0 payouts, got %d err=%v", len(history), err)
		}
	})

	t.Run("empty seller id error", func(t *testing.T) {
		svc, _, _ := setupWalletService()
		if _, err := svc.ListPayoutHistory(ctx, ""); err == nil {
			t.Fatal("expected error for empty seller id")
		}
	})
}
