package handler_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
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

func adminCtx() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: "admin-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: []string{"admin"},
	})
}

func refundReq(paymentID, refundID string, amount int64) *paymentv1.RefundPaymentRequest {
	return &paymentv1.RefundPaymentRequest{PaymentId: paymentID, RefundId: refundID, Amount: amount, Reason: "r"}
}

func TestRefundPayment_ErrorCodes(t *testing.T) {
	f := newLedgerFixture(t, 0)
	tx := f.payAndCredit(t)
	seller := userCtx("seller-L")

	for _, id := range []string{"", "has space", strings.Repeat("x", 65)} {
		_, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), id, 100000))
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("refund id %q: want InvalidArgument, got %v", id, err)
		}
	}
	got, _ := f.h.GetPayment(seller, &paymentv1.GetPaymentRequest{Id: tx.GetId()})
	if got.GetTransaction().GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID || len(got.GetTransaction().GetRefunds()) != 0 {
		t.Fatalf("an invalid refund id wrote something: %v", got)
	}

	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R", 200000)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R", 200000)); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R", 100000)); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("same id other amount: want AlreadyExists, got %v", err)
	}
	_, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R2", 300001))
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "refund amount exceeds the refundable remainder" {
		t.Fatalf("above remainder: got %v", err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R2", 300000)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R3", 1)); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("refund of a REFUNDED payment: want FailedPrecondition, got %v", err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R4", 0)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("zero amount: want InvalidArgument, got %v", err)
	}
}

// A refund id used on another payment is ALREADY_EXISTS (RPC keys are global).
func TestRefundPayment_IDReusedOnAnotherPayment(t *testing.T) {
	h, repo, _, orders := setupHandlerTest()
	orders.orders["order-1"].SellerId = "seller-1"
	orders.orders["order-2"] = &orderv1.Order{Id: "order-2", BuyerId: "buyer-1", SellerId: "seller-1", TotalAmount: 100000}
	var ids []string
	for _, o := range []string{"order-1", "order-2"} {
		tx, err := repo.CreateTransaction(context.Background(), repository.PaymentTransaction{OrderID: o, BuyerID: "buyer-1", Amount: 100000, Status: repository.PaymentStatusPaid})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tx.ID)
	}
	seller := principalCtx("seller-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER)
	if _, err := h.RefundPayment(seller, refundReq(ids[0], "R", 1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.RefundPayment(seller, refundReq(ids[1], "R", 1000)); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("want AlreadyExists, got %v", err)
	}
}

func TestGetPayment_ReadersAndWireRefunds(t *testing.T) {
	f := newLedgerFixture(t, 0)
	tx := f.payAndCredit(t)
	seller := userCtx("seller-L")
	if _, err := f.h.RefundPayment(seller, &paymentv1.RefundPaymentRequest{PaymentId: tx.GetId(), RefundId: "R1", Amount: 200000, Reason: "damaged"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R2", 100000)); err != nil {
		t.Fatal(err)
	}

	for name, ctx := range map[string]context.Context{"buyer": userCtx("buyer-L"), "seller": seller, "admin": adminCtx()} {
		res, err := f.h.GetPayment(ctx, &paymentv1.GetPaymentRequest{OrderId: "order-L"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		p := res.GetTransaction()
		if p.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_REFUNDED || p.GetRefundedAmount() != 300000 || len(p.GetRefunds()) != 2 {
			t.Fatalf("%s: %v", name, p)
		}
		r1, r2 := p.GetRefunds()[0], p.GetRefunds()[1]
		if r1.GetId() != "rpc:R1" || r1.GetSourceId() != "R1" || r1.GetSource() != paymentv1.PaymentRefundSource_PAYMENT_REFUND_SOURCE_SELLER_OR_ADMIN ||
			r1.GetRequestedAmount() != 200000 || r1.GetAmount() != 200000 || r1.GetReason() != "damaged" || r1.GetCreatedAt() == nil {
			t.Fatalf("%s: first refund %v", name, r1)
		}
		if r2.GetId() != "rpc:R2" || r2.GetSourceId() != "R2" || r2.GetAmount() != 100000 {
			t.Fatalf("%s: second refund %v", name, r2)
		}
	}
	for name, ctx := range map[string]context.Context{
		"another seller": userCtx("seller-X"),
		"service":        interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{Id: "seller-L", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE}),
	} {
		if _, err := f.h.GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: tx.GetId()}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s: want PermissionDenied, got %v", name, err)
		}
	}
	// The RefundPayment response carries the refunds too.
	res, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), "R1", 200000))
	if err != nil || len(res.GetTransaction().GetRefunds()) != 2 || res.GetTransaction().GetRefundedAmount() != 300000 {
		t.Fatalf("refund response: %v %v", res, err)
	}
}

type failingOrderClient struct{}

func (failingOrderClient) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return nil, status.Error(codes.Unavailable, "team-order down")
}

// A team-order lookup failure never lets a non-buyer through.
func TestGetPayment_SellerLookupFailureIsNotSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	payments := repository.NewInMemoryPaymentRepository()
	ledger := repository.NewInMemoryLedgerRepository()
	svc := service.NewPaymentService(payments, repository.NewInMemoryWalletRepository(), failingOrderClient{}, logger,
		service.WithSettlementLedger(repository.NewInMemorySettlementLedger(payments, ledger)))
	h := handler.NewPaymentHandler(svc, logger)
	tx, err := payments.CreateTransaction(context.Background(), repository.PaymentTransaction{OrderID: "o", BuyerID: "b", Amount: 10, Status: repository.PaymentStatusPaid})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.GetPayment(userCtx("seller-L"), &paymentv1.GetPaymentRequest{Id: tx.ID})
	if code := status.Code(err); code == codes.OK || code == codes.PermissionDenied {
		t.Fatalf("lookup failure: want a NotFound/Internal error, got %v", err)
	}
	if _, err := h.GetPayment(userCtx("b"), &paymentv1.GetPaymentRequest{Id: tx.ID}); err != nil {
		t.Fatalf("the buyer needs no lookup: %v", err)
	}
}

func TestListLedgerEntries_References(t *testing.T) {
	f := newLedgerFixture(t, 0)
	tx := f.payAndCredit(t)
	seller := userCtx("seller-L")
	for id, amount := range map[string]int64{"R1": 200000, "R2": 100000} {
		if _, err := f.h.RefundPayment(seller, refundReq(tx.GetId(), id, amount)); err != nil {
			t.Fatal(err)
		}
	}
	res, err := f.h.ListLedgerEntries(seller, &paymentv1.ListLedgerEntriesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	for _, e := range res.GetEntries() {
		refs[e.GetReferenceId()] = e.GetType()
	}
	want := map[string]string{tx.GetId(): repository.LedgerTypeOrderSettlement, "rpc:R1": repository.LedgerTypeRefundDeduction, "rpc:R2": repository.LedgerTypeRefundDeduction}
	if len(refs) != len(want) {
		t.Fatalf("references %v, want %v", refs, want)
	}
	for k, v := range want {
		if refs[k] != v {
			t.Fatalf("references %v, want %v", refs, want)
		}
	}
}
