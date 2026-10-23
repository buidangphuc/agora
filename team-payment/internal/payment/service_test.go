package payment_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

type orderStub struct {
	orders map[string]*orderv1.Order
}

func (m *orderStub) GetOrder(_ context.Context, req *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	o, ok := m.orders[req.GetId()]
	if !ok {
		return nil, errors.New("order not found")
	}
	return &orderv1.GetOrderResponse{Order: o}, nil
}

func (m *orderStub) UpdateOrderStatus(_ context.Context, _ *orderv1.UpdateOrderStatusRequest, _ ...grpc.CallOption) (*orderv1.UpdateOrderStatusResponse, error) {
	return nil, errors.New("UpdateOrderStatus must not be called")
}

type rig struct {
	svc      *payment.Service
	payments *fake.InMemoryPaymentRepository
	writer   *fake.InMemoryTxWriter
	orders   *orderStub
}

func provider(t *testing.T, adapters string) payment.Provider {
	t.Helper()
	set, err := mockdata.New(sdkconfig.MockData{Adapters: adapters}, "payment-provider")
	if err != nil {
		t.Fatalf("mockdata.New: %v", err)
	}
	return payment.NewProvider(set)
}

func newRig(t *testing.T, adapters string) *rig {
	t.Helper()
	payments := fake.NewInMemoryPaymentRepository()
	ledger := fake.NewInMemoryLedgerRepository()
	writer := fake.NewInMemoryTxWriter(payments)
	orders := &orderStub{orders: map[string]*orderv1.Order{}}
	svc, err := payment.NewService(
		payments,
		fake.NewInMemoryWalletRepository(),
		orders,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		writer,
		ledger,
		fake.NewInMemorySettlementLedger(payments, ledger),
		payment.WithProvider(provider(t, adapters)),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &rig{svc: svc, payments: payments, writer: writer, orders: orders}
}

func (r *rig) pending(t *testing.T, orderID string) store.PaymentTransaction {
	t.Helper()
	r.orders.orders[orderID] = &orderv1.Order{Id: orderID, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING}
	tx, err := r.payments.CreateTransaction(context.Background(), store.PaymentTransaction{
		OrderID: orderID,
		BuyerID: "buyer-1",
		Amount:  300000,
		Status:  store.PaymentStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}
	return tx
}

func TestNewServiceRequiresTxWriter(t *testing.T) {
	payments := fake.NewInMemoryPaymentRepository()
	ledger := fake.NewInMemoryLedgerRepository()
	svc, err := payment.NewService(
		payments,
		fake.NewInMemoryWalletRepository(),
		&orderStub{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		ledger,
		fake.NewInMemorySettlementLedger(payments, ledger),
	)
	if err == nil {
		t.Fatal("scenario 'The service refuses a missing transaction writer': expected an error for a nil transaction writer")
	}
	if svc != nil {
		t.Fatalf("expected a nil service on error, got %v", svc)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "transaction writer") {
		t.Fatalf("error must name the transaction writer, got %q", err)
	}
}

func TestNewServiceRequiresLedgerAndSettlementLedger(t *testing.T) {
	payments := fake.NewInMemoryPaymentRepository()
	ledger := fake.NewInMemoryLedgerRepository()
	writer := fake.NewInMemoryTxWriter(payments)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := payment.NewService(payments, fake.NewInMemoryWalletRepository(), &orderStub{}, logger, writer, nil, fake.NewInMemorySettlementLedger(payments, ledger)); err == nil {
		t.Error("scenario 'The service refuses a missing transaction writer': nil ledger must be refused")
	}
	if _, err := payment.NewService(payments, fake.NewInMemoryWalletRepository(), &orderStub{}, logger, writer, ledger, nil); err == nil {
		t.Error("scenario 'The service refuses a missing transaction writer': nil settlement ledger must be refused")
	}
}

func TestMockDataProviderSettlesThroughRealPath(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, "payment-provider")
	tx := r.pending(t, "order-fx-1")

	updated, ok, _, err := r.svc.ProcessMockPayment(ctx, tx.ID, true)
	if err != nil || !ok {
		t.Fatalf("scenario 'The fixture provider settles through the real path': ok=%v err=%v", ok, err)
	}
	if updated.Status != store.PaymentStatusPaid {
		t.Fatalf("returned status = %v, want PAID", updated.Status)
	}
	stored, err := r.payments.GetTransaction(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != store.PaymentStatusPaid {
		t.Fatalf("stored status = %v, want PAID", stored.Status)
	}
	if want := "mock-" + tx.ID; stored.ProviderReference != want {
		t.Fatalf("provider_reference = %q, want %q", stored.ProviderReference, want)
	}
	rows := r.writer.Rows()
	if len(rows) != 1 || rows[0].EventType != events.PaymentSettledEventType || rows[0].AggregateID != "order-fx-1" {
		t.Fatalf("want exactly 1 PaymentSettled row for the order, got %+v", rows)
	}
}

func TestMockDataProviderFailureWritesNoOutbox(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, "payment-provider")
	tx := r.pending(t, "order-fx-2")

	updated, ok, _, err := r.svc.ProcessMockPayment(ctx, tx.ID, false)
	if err != nil {
		t.Fatalf("scenario 'A simulated failure sets FAILED and writes no outbox row': %v", err)
	}
	if ok {
		t.Fatal("ok must be false for a simulated failure")
	}
	if updated.Status != store.PaymentStatusFailed {
		t.Fatalf("returned status = %v, want FAILED", updated.Status)
	}
	stored, _ := r.payments.GetTransaction(ctx, tx.ID)
	if stored.Status != store.PaymentStatusFailed {
		t.Fatalf("stored status = %v, want FAILED", stored.Status)
	}
	if n := len(r.writer.Rows()); n != 0 {
		t.Fatalf("want 0 outbox rows, got %d", n)
	}
}

func TestLiveProviderRefusesMockPayment(t *testing.T) {
	ctx := context.Background()
	for _, simulate := range []bool{true, false} {
		r := newRig(t, "")
		tx := r.pending(t, "order-live")

		_, _, _, err := r.svc.ProcessMockPayment(ctx, tx.ID, simulate)
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("scenario 'Without the adapter the mock RPC is refused' (simulate=%v): code = %v err=%v, want FailedPrecondition", simulate, status.Code(err), err)
		}
		if !strings.Contains(err.Error(), "mock payments are disabled") {
			t.Fatalf("error text = %q", err)
		}
		stored, _ := r.payments.GetTransaction(ctx, tx.ID)
		if stored.Status != store.PaymentStatusPending {
			t.Fatalf("transaction must be unchanged, got %v", stored.Status)
		}
		if n := len(r.writer.Rows()); n != 0 {
			t.Fatalf("want 0 outbox rows, got %d", n)
		}
	}
}

func TestService_Settle_WritesPaymentAndOutboxAtomically(t *testing.T) {
	ctx := context.Background()

	t.Run("settle writes payment=PAID and a PaymentSettled outbox row in one tx", func(t *testing.T) {
		r := newRig(t, "payment-provider")
		tx := r.pending(t, "order-settle-1")

		updated, ok, _, err := r.svc.ProcessMockPayment(ctx, tx.ID, true)
		if err != nil || !ok {
			t.Fatalf("ProcessMockPayment: ok=%v err=%v", ok, err)
		}
		if updated.Status != store.PaymentStatusPaid {
			t.Fatalf("status = %v, want PAID", updated.Status)
		}
		rows := r.writer.Rows()
		if len(rows) != 1 {
			t.Fatalf("want 1 outbox row, got %d", len(rows))
		}
		row := rows[0]
		if row.EventType != events.PaymentSettledEventType || row.AggregateID != "order-settle-1" || row.EventID == "" {
			t.Fatalf("unexpected row %+v", row)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetEventId() != row.EventID || env.GetType() != events.PaymentSettledEventType {
			t.Fatalf("envelope mismatch: %v", &env)
		}
		var settled paymentv1.PaymentSettled
		if err := proto.Unmarshal(env.GetPayload(), &settled); err != nil {
			t.Fatalf("unmarshal PaymentSettled: %v", err)
		}
		if settled.GetOrderId() != "order-settle-1" || settled.GetPaymentId() != tx.ID ||
			settled.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
			t.Fatalf("unexpected PaymentSettled %+v", &settled)
		}
	})

	t.Run("a failed write leaves the payment PENDING and no outbox row", func(t *testing.T) {
		r := newRig(t, "payment-provider")
		r.writer.FailWrite = true
		tx := r.pending(t, "order-settle-2")

		if _, _, _, err := r.svc.ProcessMockPayment(ctx, tx.ID, true); err == nil {
			t.Fatal("expected an error when the transactional write fails")
		}
		stored, _ := r.payments.GetTransaction(ctx, tx.ID)
		if stored.Status != store.PaymentStatusPending {
			t.Fatalf("payment must still be PENDING, got %v", stored.Status)
		}
		if n := len(r.writer.Rows()); n != 0 {
			t.Fatalf("want 0 outbox rows, got %d", n)
		}
	})
}

type brokenPayoutStore struct {
	*fake.InMemoryWalletRepository
}

func (brokenPayoutStore) CreatePayoutRequest(context.Context, store.PayoutRequest) (store.PayoutRequest, error) {
	return store.PayoutRequest{}, errors.New("payout store down")
}

type brokenCompensation struct {
	*fake.InMemoryLedgerRepository
}

func (brokenCompensation) AppendEntry(context.Context, store.LedgerEntry) (store.LedgerEntry, error) {
	return store.LedgerEntry{}, errors.New("ledger append down")
}

func TestPayoutCompensationFailureLogDropsRawSellerID(t *testing.T) {
	ctx := context.Background()
	const seller = "seller-raw-id-7f3a"

	payments := fake.NewInMemoryPaymentRepository()
	ledger := fake.NewInMemoryLedgerRepository()
	if _, err := ledger.AppendEntry(ctx, store.LedgerEntry{
		SellerID: seller, Type: store.LedgerTypeOrderSettlement, Amount: 1000000, Status: store.LedgerStatusCompleted,
	}); err != nil {
		t.Fatalf("seed credit: %v", err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	svc, err := payment.NewService(
		payments,
		brokenPayoutStore{fake.NewInMemoryWalletRepository()},
		&orderStub{},
		logger,
		fake.NewInMemoryTxWriter(payments),
		brokenCompensation{ledger},
		fake.NewInMemorySettlementLedger(payments, ledger),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.RequestPayout(ctx, seller, 100000, "VCB", "123", "ACC"); err == nil {
		t.Fatal("expected RequestPayout to fail when the payout record cannot be stored")
	}
	out := buf.String()
	if !strings.Contains(out, "payout compensation failed") {
		t.Fatalf("expected the compensation error to be logged, got %q", out)
	}
	if !strings.Contains(out, "ledger_entry_id") || !strings.Contains(out, "ledger append down") {
		t.Fatalf("log must keep ledger_entry_id and err, got %q", out)
	}
	if strings.Contains(out, seller) || strings.Contains(out, "seller_id") {
		t.Fatalf("log must not carry the raw seller id, got %q", out)
	}
}
