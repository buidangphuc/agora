package rpc_test

import (
	"context"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
)

type errOrderClient struct{ err error }

func (c errOrderClient) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return nil, c.err
}

// grpcx.Status passes a gRPC status error through unchanged, so a team-order status wrapped
// into a domain error must not reach the caller with team-order's code and text: the client
// keeps seeing the payment service's own words.
func TestOrderLookupFailureKeepsClientText(t *testing.T) {
	cases := []struct {
		name     string
		upstream error
		code     codes.Code
		msg      string
	}{
		{"order down", status.Error(codes.Unavailable, "team-order down"), codes.Unavailable, "order service unavailable"},
		{"order refuses", status.Error(codes.PermissionDenied, "secret upstream detail"), codes.NotFound, "order not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.DiscardHandler)
			payments := store.NewInMemoryPaymentRepository()
			ledger := store.NewInMemoryLedgerRepository()
			svc := payment.NewService(payments, store.NewInMemoryWalletRepository(), errOrderClient{tc.upstream}, logger,
				payment.WithSettlementLedger(store.NewInMemorySettlementLedger(payments, ledger)))
			h := rpc.NewPaymentHandler(svc, logger)
			tx, err := payments.CreateTransaction(context.Background(), store.PaymentTransaction{OrderID: "o", BuyerID: "b", Amount: 10, Status: store.PaymentStatusPaid})
			if err != nil {
				t.Fatal(err)
			}

			// GetPayment by a non-buyer looks the seller up.
			_, err = h.GetPayment(userCtx("seller-L"), &paymentv1.GetPaymentRequest{Id: tx.ID})
			if status.Code(err) != tc.code || status.Convert(err).Message() != tc.msg {
				t.Errorf("GetPayment: got %v, want %s %q", err, tc.code, tc.msg)
			}
			// RefundPayment by a non-admin looks the seller up too.
			_, err = h.RefundPayment(userCtx("seller-L"), refundReq(tx.ID, "R1", 5))
			if status.Code(err) != tc.code || status.Convert(err).Message() != tc.msg {
				t.Errorf("RefundPayment: got %v, want %s %q", err, tc.code, tc.msg)
			}
		})
	}

	// CreatePayment has no unavailable case: any lookup failure is "order not found".
	logger := slog.New(slog.DiscardHandler)
	svc := payment.NewService(store.NewInMemoryPaymentRepository(), store.NewInMemoryWalletRepository(),
		errOrderClient{status.Error(codes.Unavailable, "team-order down")}, logger)
	_, err := rpc.NewPaymentHandler(svc, logger).CreatePayment(userCtx("b"), &paymentv1.CreatePaymentRequest{OrderId: "o"})
	if status.Code(err) != codes.NotFound || status.Convert(err).Message() != "order not found" {
		t.Errorf("CreatePayment: got %v, want NotFound \"order not found\"", err)
	}
}
