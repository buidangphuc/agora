package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

const leakSecret = "pq: connection refused host=10.1.2.3 password=hunter2 password=secret"

// Scenario: Without the adapter the mock RPC is refused
func TestProcessMockPayment_DisabledByDefault(t *testing.T) {
	st := newStores()
	svc := st.service(t, nil, payment.WithProvider(liveProvider()))
	h := newHandler(t, svc, permissiveAudit)

	tx, err := st.payments.CreateTransaction(context.Background(), store.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 1000, Currency: "VND",
		Status: store.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, simulate := range []bool{true, false} {
		_, err = h.ProcessMockPayment(principalCtx("buyer-1", auth.TypeUser, "admin"), &paymentv1.ProcessMockPaymentRequest{
			TransactionId: tx.ID, SimulateSuccess: simulate,
		})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("want FailedPrecondition, got %v", err)
		}
	}
	got, _ := st.payments.GetTransaction(context.Background(), tx.ID)
	if got.Status != store.PaymentStatusPending {
		t.Fatalf("transaction changed while mock payments disabled: %v", got.Status)
	}
	if n := len(st.writer.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: Without the adapter the mock RPC is refused
func TestProcessMockPayment_DisabledByDefault_RefusedBeforeOwnershipAndLookup(t *testing.T) {
	st := newStores()
	svc := st.service(t, nil, payment.WithProvider(liveProvider()))
	h := newHandler(t, svc, permissiveAudit)

	tx, err := st.payments.CreateTransaction(context.Background(), store.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 1000, Currency: "VND",
		Status: store.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		ctx  context.Context
		id   string
	}{
		{"non-buyer non-admin", principalCtx("buyer-2", auth.TypeUser, "payment:write"), tx.ID},
		{"unknown transaction id", principalCtx("buyer-1", auth.TypeUser), "non-existent-tx"},
	}
	for _, c := range cases {
		for _, simulate := range []bool{true, false} {
			_, err := h.ProcessMockPayment(c.ctx, &paymentv1.ProcessMockPaymentRequest{TransactionId: c.id, SimulateSuccess: simulate})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("%s simulate=%v: want FailedPrecondition, got %v", c.name, simulate, err)
			}
		}
	}
	got, _ := st.payments.GetTransaction(context.Background(), tx.ID)
	if got.Status != store.PaymentStatusPending {
		t.Fatalf("transaction changed while mock payments disabled: %v", got.Status)
	}
	if n := len(st.writer.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: The local stack settles a checkout through the mock payment
func TestMockDataProviderSettlesThroughRealPath(t *testing.T) {
	st := newStores()
	orders := &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-1": {Id: "order-1", BuyerId: "buyer-1", SellerId: "seller-1", TotalAmount: 100000, Currency: "VND", Status: orderv1.OrderStatus_ORDER_STATUS_PENDING},
	}}
	h := newHandler(t, st.service(t, orders), permissiveAudit)
	buyer := principalCtx("buyer-1", auth.TypeUser)

	created, err := h.CreatePayment(buyer, &paymentv1.CreatePaymentRequest{OrderId: "order-1", Method: paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_CARD})
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.ProcessMockPayment(buyer, &paymentv1.ProcessMockPaymentRequest{TransactionId: created.GetTransaction().GetId(), SimulateSuccess: true})
	if err != nil || !res.GetSuccess() || res.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
		t.Fatalf("pay: %v %v", res, err)
	}
	if res.GetTransaction().GetProviderReference() != "mock-"+created.GetTransaction().GetId() {
		t.Fatalf("provider reference = %q", res.GetTransaction().GetProviderReference())
	}
	if n := len(st.writer.Rows()); n != 1 {
		t.Fatalf("outbox rows = %d, want 1 written in the settle transaction", n)
	}
}

// Scenario: A user who is neither the buyer nor an admin cannot use the mock path
func TestProcessMockPayment_Errors(t *testing.T) {
	st := newStores()
	h := newHandler(t, st.service(t, &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-1": {Id: "order-1", BuyerId: "buyer-1", TotalAmount: 1000, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING},
	}}), permissiveAudit)
	tx, err := st.payments.CreateTransaction(context.Background(), store.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 1000, Currency: "VND", Status: store.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	other := principalCtx("buyer-2", auth.TypeUser, "payment:write")
	for _, simulate := range []bool{true, false} {
		_, err := h.ProcessMockPayment(other, &paymentv1.ProcessMockPaymentRequest{TransactionId: tx.ID, SimulateSuccess: simulate})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("simulate=%v: want PermissionDenied, got %v", simulate, err)
		}
	}
	got, _ := st.payments.GetTransaction(context.Background(), tx.ID)
	if got.Status != store.PaymentStatusPending {
		t.Fatalf("transaction changed: %v", got.Status)
	}
	if n := len(st.writer.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
	owner := principalCtx("buyer-1", auth.TypeUser)
	if _, err := h.ProcessMockPayment(owner, &paymentv1.ProcessMockPaymentRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing transaction id: want InvalidArgument, got %v", err)
	}
	if _, err := h.ProcessMockPayment(owner, &paymentv1.ProcessMockPaymentRequest{TransactionId: "non-existent-tx", SimulateSuccess: true}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown transaction: want NotFound, got %v", err)
	}
}

type failingPayments struct {
	*fake.InMemoryPaymentRepository
	err error
}

func (f failingPayments) GetTransaction(context.Context, string) (store.PaymentTransaction, error) {
	return store.PaymentTransaction{}, f.err
}

func (f failingPayments) GetTransactionByOrderID(context.Context, string) (store.PaymentTransaction, error) {
	return store.PaymentTransaction{}, f.err
}

func (f failingPayments) CreateTransaction(context.Context, store.PaymentTransaction) (store.PaymentTransaction, error) {
	return store.PaymentTransaction{}, f.err
}

func failingHandler(t *testing.T, err error, logger *slog.Logger) *rpc.Handler {
	t.Helper()
	st := newStores()
	st.payRepo = failingPayments{InMemoryPaymentRepository: st.payments, err: err}
	return newHandlerLogged(t, st.service(t, nil), permissiveAudit, logger)
}

// Scenario: An unexpected error is not echoed
func TestInternalErrorsAreNotEchoed(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	h := failingHandler(t, errors.New(leakSecret), logger)
	ctx := principalCtx("buyer-1", auth.TypeUser)

	_, err1 := h.GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: "tx-1"})
	_, err2 := h.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{TransactionId: "tx-1", SimulateSuccess: true})
	for i, err := range []error{err1, err2} {
		if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
			t.Fatalf("case %d: want Internal \"internal error\", got %v", i, err)
		}
		for _, secret := range []string{"hunter2", "10.1.2.3", "password=secret"} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("case %d: internal error leaked cause: %v", i, err)
			}
		}
	}
	if !strings.Contains(logs.String(), "hunter2") {
		t.Fatalf("cause must be logged server-side, logs: %s", logs.String())
	}
}

// Scenario: A unique-key violation is AlreadyExists
func TestUniqueViolationIsAlreadyExists(t *testing.T) {
	st := newStores()
	st.payRepo = failingPayments{InMemoryPaymentRepository: st.payments, err: &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}}
	orders := &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-1": {Id: "order-1", BuyerId: "buyer-1", TotalAmount: 1000, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING},
	}}
	h := newHandler(t, st.service(t, orders), permissiveAudit)
	_, err := h.CreatePayment(principalCtx("buyer-1", auth.TypeUser), &paymentv1.CreatePaymentRequest{OrderId: "order-1"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("want AlreadyExists, got %v", err)
	}
	if strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("driver text leaked: %v", err)
	}
}

// Scenario: A cancelled or timed-out call keeps its code
func TestContextErrorsKeepTheirCodes(t *testing.T) {
	ctx := principalCtx("buyer-1", auth.TypeUser)
	for _, c := range []struct {
		err  error
		want codes.Code
	}{
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		h := failingHandler(t, c.err, discardLogger())
		_, err := h.GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: "tx-1"})
		if status.Code(err) != c.want {
			t.Fatalf("%v: want %v, got %v", c.err, c.want, err)
		}
	}
}
