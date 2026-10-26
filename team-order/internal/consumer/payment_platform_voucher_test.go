package consumer_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/repository"
)

type platformCommitter struct {
	mu        sync.Mutex
	committed []string
	failID    string
	failErr   error
}

func (f *platformCommitter) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failID != "" && in.GetReservationId() == f.failID {
		return nil, f.failErr
	}
	f.committed = append(f.committed, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func paidEnvelope(t *testing.T, evt, orderID string) *paymentv1.PaymentSettled {
	t.Helper()
	return &paymentv1.PaymentSettled{OrderId: orderID, Status: paymentv1.PaymentStatus_PAYMENT_STATUS_PAID}
}

// Scenario: Multishop checkout: First paid order commits the platform voucher
func TestSettleCommitsPlatformVoucherOnFirstPaid(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_a": {ID: "ord_a", Status: repository.OrderStatusPending, CheckoutID: "chk_1", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000},
		"ord_b": {ID: "ord_b", Status: repository.OrderStatusPending, CheckoutID: "chk_1", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000},
	}}
	promo := &platformCommitter{}
	c := consumer.NewPaymentConsumer(store, nil, nil, consumer.WithVoucherCommitter(promo))

	if err := c.HandleEnvelope(ctx, envelopeFor(t, "evt_pa", paidEnvelope(t, "evt_pa", "ord_a"))); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if len(promo.committed) != 1 || promo.committed[0] != "platform:chk_1" {
		t.Fatalf("expected exactly one commit of platform:chk_1, got %v", promo.committed)
	}
	o, _ := store.GetOrder(ctx, "ord_a")
	if o.Status != repository.OrderStatusPaid {
		t.Fatalf("expected order PAID, got %d", o.Status)
	}
}

// Scenario: Multishop checkout: An order without a platform voucher makes no platform commit
func TestSettleNoPlatformCommitWithoutPlatformVoucher(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_s": {ID: "ord_s", Status: repository.OrderStatusPending, CheckoutID: "chk_2", VoucherCode: "SHOP5", DiscountAmount: 5000},
	}}
	promo := &platformCommitter{}
	c := consumer.NewPaymentConsumer(store, nil, nil, consumer.WithVoucherCommitter(promo))

	if err := c.HandleEnvelope(ctx, envelopeFor(t, "evt_s", paidEnvelope(t, "evt_s", "ord_s"))); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if len(promo.committed) != 1 || promo.committed[0] != "ord_s" {
		t.Fatalf("expected only the shop commit under the order id, got %v", promo.committed)
	}
}

// Scenario: Multishop checkout: A platform commit failure is retried
func TestSettlePlatformCommitErrorRetried(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_f": {ID: "ord_f", Status: repository.OrderStatusPending, CheckoutID: "chk_3", PlatformVoucherCode: "SAVE10", PlatformDiscountAmount: 5000},
	}}
	promo := &platformCommitter{failID: "platform:chk_3", failErr: errors.New("transport down")}
	c := consumer.NewPaymentConsumer(store, nil, nil, consumer.WithVoucherCommitter(promo))

	env := envelopeFor(t, "evt_f", paidEnvelope(t, "evt_f", "ord_f"))
	if err := c.HandleEnvelope(ctx, env); err == nil {
		t.Fatal("expected the platform commit failure to be returned for retry")
	}
}
