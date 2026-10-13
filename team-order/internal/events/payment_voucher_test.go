package events_test

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

// fakeVoucherCommitter records the reservation ids it was asked to commit so the
// test can prove the settle-time commit fired exactly once with the order id.
type fakeVoucherCommitter struct {
	mu        sync.Mutex
	committed []string
}

func (f *fakeVoucherCommitter) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.committed = append(f.committed, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

// A settled order that carries a voucher commits its voucher hold (keyed by the
// order id), exactly once even across a duplicate delivery.
func TestPaymentConsumer_CommitsVoucherOnSettle(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_v": {ID: "ord_v", Status: store.OrderStatusPending, VoucherCode: "SAVE30", DiscountAmount: 30000},
	}}
	promo := &fakeVoucherCommitter{}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil, events.WithVoucherCommitter(promo))

	env := envelopeFor(t, "evt_v", &paymentv1.PaymentSettled{
		OrderId: "ord_v",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	// Duplicate delivery is deduped before apply — must not commit twice.
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("duplicate consume failed: %v", err)
	}

	if len(promo.committed) != 1 {
		t.Fatalf("expected exactly 1 voucher commit, got %d", len(promo.committed))
	}
	if promo.committed[0] != "ord_v" {
		t.Fatalf("expected commit keyed by order id ord_v, got %q", promo.committed[0])
	}
	o, _ := fake.GetOrder(ctx, "ord_v")
	if o.Status != store.OrderStatusPaid {
		t.Fatalf("expected order PAID, got status %d", o.Status)
	}
}

// An order with no voucher never triggers a commit.
func TestPaymentConsumer_NoVoucher_NoCommit(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_p": {ID: "ord_p", Status: store.OrderStatusPending},
	}}
	promo := &fakeVoucherCommitter{}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil, events.WithVoucherCommitter(promo))

	env := envelopeFor(t, "evt_p", &paymentv1.PaymentSettled{
		OrderId: "ord_p",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if len(promo.committed) != 0 {
		t.Fatalf("expected no voucher commit for a voucher-less order, got %d", len(promo.committed))
	}
}
