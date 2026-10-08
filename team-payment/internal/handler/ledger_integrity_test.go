package handler_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

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

type ledgerFixture struct {
	h        *handler.PaymentHandler
	svc      *service.PaymentService
	payments *repository.InMemoryPaymentRepository
	ledger   *repository.InMemoryLedgerRepository
	wallets  *repository.InMemoryWalletRepository
}

// newLedgerFixture wires the handler over in-memory stores with a 7-day hold and order
// "order-L" (seller "seller-L", buyer "buyer-L").
func newLedgerFixture(t *testing.T, hold time.Duration) ledgerFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := ledgerFixture{
		payments: repository.NewInMemoryPaymentRepository(),
		ledger:   repository.NewInMemoryLedgerRepository(),
		wallets:  repository.NewInMemoryWalletRepository(),
	}
	orders := &mockOrderClient{orders: map[string]*orderv1.Order{
		"order-L": {Id: "order-L", BuyerId: "buyer-L", SellerId: "seller-L", TotalAmount: 500000, Status: orderv1.OrderStatus_ORDER_STATUS_PENDING},
	}}
	f.svc = service.NewPaymentService(f.payments, f.wallets, orders, logger,
		service.WithLedgerRepo(f.ledger),
		service.WithSettlementLedger(repository.NewInMemorySettlementLedger(f.payments, f.ledger)),
		service.WithPayoutHold(hold))
	f.h = handler.NewPaymentHandler(f.svc, logger, handler.WithMockPayments(true))
	return f
}

func userCtx(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
	})
}

// payAndCredit pays order-L through the RPCs and applies its credit as the consumer would.
func (f ledgerFixture) payAndCredit(t *testing.T) *paymentv1.PaymentTransaction {
	t.Helper()
	buyer := userCtx("buyer-L")
	created, err := f.h.CreatePayment(buyer, &paymentv1.CreatePaymentRequest{OrderId: "order-L", Method: paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_CARD})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.h.ProcessMockPayment(buyer, &paymentv1.ProcessMockPaymentRequest{TransactionId: created.GetTransaction().GetId(), SimulateSuccess: true}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	if err := f.svc.CreditSettlement(context.Background(), "order-L", "seller-L", 500000); err != nil {
		t.Fatalf("credit: %v", err)
	}
	return created.GetTransaction()
}

func entriesOf(t *testing.T, f ledgerFixture, seller string) map[string][]repository.LedgerEntry {
	t.Helper()
	entries, _, err := f.ledger.ListEntries(context.Background(), seller, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]repository.LedgerEntry{}
	for _, e := range entries {
		out[e.Type] = append(out[e.Type], e)
	}
	return out
}

func TestMockPaymentWritesNoLedgerEntry(t *testing.T) {
	f := newLedgerFixture(t, 0)
	buyer := userCtx("buyer-L")
	created, err := f.h.CreatePayment(buyer, &paymentv1.CreatePaymentRequest{OrderId: "order-L"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.h.ProcessMockPayment(buyer, &paymentv1.ProcessMockPaymentRequest{TransactionId: created.GetTransaction().GetId(), SimulateSuccess: true})
	if err != nil || !res.GetSuccess() {
		t.Fatalf("pay: %v %v", res, err)
	}
	if got := entriesOf(t, f, "seller-L"); len(got) != 0 {
		t.Fatalf("ProcessMockPayment wrote ledger entries: %+v", got)
	}
}

func TestRefundDeductsCreditedSellerOnce(t *testing.T) {
	f := newLedgerFixture(t, 0)
	tx := f.payAndCredit(t)
	seller := userCtx("seller-L")

	res, err := f.h.RefundPayment(seller, &paymentv1.RefundPaymentRequest{PaymentId: tx.GetId(), Amount: 200000, Reason: "damaged"})
	if err != nil || res.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_REFUNDED {
		t.Fatalf("refund: %v %v", res, err)
	}
	_, err = f.h.RefundPayment(seller, &paymentv1.RefundPaymentRequest{PaymentId: tx.GetId(), Amount: 200000})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("second refund: want FailedPrecondition, got %v", err)
	}
	got := entriesOf(t, f, "seller-L")
	if d := got[repository.LedgerTypeRefundDeduction]; len(d) != 1 || d[0].Amount != -200000 || d[0].ReferenceID != tx.GetId() {
		t.Fatalf("deductions: %+v", d)
	}
	bal, err := f.h.GetWalletBalance(seller, &paymentv1.GetWalletBalanceRequest{})
	if err != nil || bal.GetBalance() != 300000 {
		t.Fatalf("balance: %v %v", bal, err)
	}
}

// A refund is applied even when it takes the balance negative.
func TestRefundAfterPayoutTakesBalanceNegative(t *testing.T) {
	f := newLedgerFixture(t, 0)
	tx := f.payAndCredit(t)
	seller := userCtx("seller-L")
	if _, err := f.h.RequestWalletPayout(seller, &paymentv1.RequestWalletPayoutRequest{Amount: 500000}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.RefundPayment(seller, &paymentv1.RefundPaymentRequest{PaymentId: tx.GetId(), Amount: 200000}); err != nil {
		t.Fatalf("refund: %v", err)
	}
	bal, _ := f.h.GetWalletBalance(seller, &paymentv1.GetWalletBalanceRequest{})
	if bal.GetBalance() != -200000 {
		t.Fatalf("balance = %d, want -200000", bal.GetBalance())
	}
}

func TestPayoutRefusalsHeldVsInsufficient(t *testing.T) {
	f := newLedgerFixture(t, 7*24*time.Hour)
	f.payAndCredit(t)
	seller := userCtx("seller-L")

	checkHeld := func(name string, err error) {
		t.Helper()
		st, _ := status.FromError(err)
		if st.Code() != codes.FailedPrecondition || !strings.HasPrefix(st.Message(), "amount is held until ") ||
			!strings.HasSuffix(st.Message(), " (refund window)") {
			t.Fatalf("%s: want held FailedPrecondition, got %v", name, err)
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(st.Message(), "amount is held until "), " (refund window)")
		until, perr := time.Parse(time.RFC3339, stamp)
		if perr != nil || until.Sub(time.Now().Add(7*24*time.Hour)).Abs() > 5*time.Second {
			t.Fatalf("%s: release instant %q (%v) not ~now+7d", name, stamp, perr)
		}
		if strings.Contains(st.Message(), "100000") || strings.Contains(st.Message(), "500000") {
			t.Fatalf("%s: message reveals an amount: %q", name, st.Message())
		}
	}

	_, err := f.h.RequestWalletPayout(seller, &paymentv1.RequestWalletPayoutRequest{Amount: 100000})
	checkHeld("RequestWalletPayout", err)
	_, err = f.h.RequestPayout(seller, &paymentv1.RequestPayoutRequest{Amount: 100000, BankCode: "VCB", AccountNumber: "1", AccountName: "A"})
	checkHeld("RequestPayout", err)

	for name, call := range map[string]func() error{
		"RequestWalletPayout": func() error {
			_, err := f.h.RequestWalletPayout(seller, &paymentv1.RequestWalletPayoutRequest{Amount: 500001})
			return err
		},
		"RequestPayout": func() error {
			_, err := f.h.RequestPayout(seller, &paymentv1.RequestPayoutRequest{Amount: 500001, BankCode: "VCB", AccountNumber: "1", AccountName: "A"})
			return err
		},
	} {
		st, _ := status.FromError(call())
		if st.Code() != codes.FailedPrecondition || st.Message() != "insufficient wallet balance" {
			t.Fatalf("%s above balance: got %v", name, st)
		}
	}

	history, err := f.h.ListPayoutHistory(seller, &paymentv1.ListPayoutHistoryRequest{})
	if err != nil || len(history.GetPayouts()) != 0 {
		t.Fatalf("a refused payout recorded a payout request: %v %v", history, err)
	}
	if p := entriesOf(t, f, "seller-L")[repository.LedgerTypePayout]; len(p) != 0 {
		t.Fatalf("a refused payout wrote ledger entries: %+v", p)
	}
	for _, rpc := range []func() (int64, error){
		func() (int64, error) {
			r, err := f.h.GetWalletBalance(seller, &paymentv1.GetWalletBalanceRequest{})
			return r.GetBalance(), err
		},
		func() (int64, error) {
			r, err := f.h.GetSellerWallet(seller, &paymentv1.GetSellerWalletRequest{})
			return r.GetWallet().GetBalance(), err
		},
	} {
		if bal, err := rpc(); err != nil || bal != 500000 {
			t.Fatalf("balance must include held proceeds: %d %v", bal, err)
		}
	}
}
