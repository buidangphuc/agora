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
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

// unavailableOrders is team-order being unreachable.
type unavailableOrders struct{ mockOrderClient }

func (unavailableOrders) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return nil, status.Error(codes.Unavailable, "team-order down")
}

type cancelHandlerFixture struct {
	h      *handler.PaymentHandler
	repo   *repository.InMemoryPaymentRepository
	writer *repository.InMemoryTxWriter
}

func newCancelHandlerFixture(oc upstream.OrderClient) cancelHandlerFixture {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewInMemoryPaymentRepository()
	w := repository.NewInMemoryTxWriter(repo)
	svc := service.NewPaymentService(repo, repository.NewInMemoryWalletRepository(), oc, logger,
		service.WithTxWriter(w),
		service.WithLedgerRepo(repository.NewInMemoryLedgerRepository()),
		service.WithSettlementLedger(repository.NewInMemorySettlementLedger(repo, repository.NewInMemoryLedgerRepository())))
	h := handler.NewPaymentHandler(svc, logger, handler.WithMockPayments(true), handler.WithAuditWriter(permissiveAudit{}))
	return cancelHandlerFixture{h: h, repo: repo, writer: w}
}

func (f cancelHandlerFixture) seed(t *testing.T, st repository.PaymentStatus) repository.PaymentTransaction {
	t.Helper()
	tx, err := f.repo.CreateTransaction(context.Background(), repository.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 100000, Currency: "VND", Status: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func cancelledOrders() *mockOrderClient {
	return &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-1": {Id: "order-1", BuyerId: "buyer-1", SellerId: "seller-1", TotalAmount: 100000, Currency: "VND",
			Status: orderv1.OrderStatus_ORDER_STATUS_CANCELLED},
	}}
}

func (f cancelHandlerFixture) assertStatus(t *testing.T, id string, want repository.PaymentStatus) {
	t.Helper()
	got, err := f.repo.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != want {
		t.Fatalf("transaction status = %v, want %v", got.Status, want)
	}
	if n := len(f.writer.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

var buyerCtx = func() context.Context {
	return principalCtx("buyer-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER)
}

// Scenario: Paying a cancelled transaction is refused
func TestProcessMockPaymentCancelledRefused(t *testing.T) {
	f := newCancelHandlerFixture(cancelledOrders())
	tx := f.seed(t, repository.PaymentStatusCancelled)
	_, err := f.h.ProcessMockPayment(buyerCtx(), &paymentv1.ProcessMockPaymentRequest{TransactionId: tx.ID, SimulateSuccess: true})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "payment cancelled" {
		t.Fatalf("want FailedPrecondition \"payment cancelled\", got %v", err)
	}
	f.assertStatus(t, tx.ID, repository.PaymentStatusCancelled)
}

// Scenario: Simulated failure on a cancelled transaction is refused
func TestProcessMockPaymentCancelledFailureRefused(t *testing.T) {
	f := newCancelHandlerFixture(cancelledOrders())
	tx := f.seed(t, repository.PaymentStatusCancelled)
	_, err := f.h.ProcessMockPayment(buyerCtx(), &paymentv1.ProcessMockPaymentRequest{TransactionId: tx.ID, SimulateSuccess: false})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	f.assertStatus(t, tx.ID, repository.PaymentStatusCancelled)
}

// Scenario: Non-owner is denied before the cancelled check
func TestProcessMockPaymentCancelledNonOwnerDenied(t *testing.T) {
	f := newCancelHandlerFixture(cancelledOrders())
	tx := f.seed(t, repository.PaymentStatusCancelled)
	for _, simulate := range []bool{true, false} {
		_, err := f.h.ProcessMockPayment(principalCtx("stranger", commonv1.PrincipalType_PRINCIPAL_TYPE_USER),
			&paymentv1.ProcessMockPaymentRequest{TransactionId: tx.ID, SimulateSuccess: simulate})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("simulate=%v: want PermissionDenied, got %v", simulate, err)
		}
	}
	f.assertStatus(t, tx.ID, repository.PaymentStatusCancelled)
}

// Scenario: Settle is not attempted when team-order is unreachable
func TestProcessMockPaymentOrderUnavailableNoSettle(t *testing.T) {
	f := newCancelHandlerFixture(&unavailableOrders{})
	tx := f.seed(t, repository.PaymentStatusPending)
	_, err := f.h.ProcessMockPayment(buyerCtx(), &paymentv1.ProcessMockPaymentRequest{TransactionId: tx.ID, SimulateSuccess: true})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	f.assertStatus(t, tx.ID, repository.PaymentStatusPending)
}

// Scenario: Cancelled order cannot open a new transaction
func TestCreatePaymentCancelledOrderRefused(t *testing.T) {
	f := newCancelHandlerFixture(cancelledOrders())
	_, err := f.h.CreatePayment(buyerCtx(), &paymentv1.CreatePaymentRequest{OrderId: "order-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if _, err := f.repo.GetTransactionByOrderID(context.Background(), "order-1"); err == nil {
		t.Fatal("a transaction row exists for the cancelled order")
	}
}

// Scenario: Buyer cannot trigger the late-payment refund by RPC
func TestRefundPaymentBuyerDeniedOnCancelledOrderPayment(t *testing.T) {
	f := newCancelHandlerFixture(cancelledOrders())
	tx := f.seed(t, repository.PaymentStatusPaid)
	_, err := f.h.RefundPayment(buyerCtx(), &paymentv1.RefundPaymentRequest{
		PaymentId: tx.ID, RefundId: "cancel:order-1", Amount: 100000, Reason: "late",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	got, _ := f.repo.GetTransaction(context.Background(), tx.ID)
	if got.RefundedAmount != 0 || got.Status != repository.PaymentStatusPaid {
		t.Fatalf("refund state changed: %+v", got)
	}
}
