package consumer_test

import (
	"context"
	"testing"
	"time"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// Scenario: Order expiry: A payment settling after the expiry changes nothing in team-order
// The order is expired by the real lifecycle sweep (not seeded as Cancelled), then
// a PaymentSettled PAID arrives: the order stays Cancelled with its expiry reason,
// no voucher commit, no OrderPaid outbox row, and the event is acknowledged
// (including on redelivery).
func TestLateSettleAfterExpiryLeavesOrderCancelled(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository(
		repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		repository.WithInMemoryOutbox(outbox))
	svc := service.NewOrderService(orders, repository.NewInMemoryCartRepository(), nil, nil,
		upstreamtest.NewDomain(map[string]int32{}), nil, nil)

	due := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	placed, err := orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer_1", SellerID: "s1",
		TotalAmount: 3000, Currency: "VND", VoucherCode: "SAVE",
		PaymentMethod: int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), PaymentDueAt: &due,
		Items: []repository.OrderItem{{ListingID: "lst_1", Quantity: 1, UnitPrice: 3000}}})
	if err != nil {
		t.Fatal(err)
	}

	svc.SweepLifecycle(ctx, due.Add(time.Second))
	expired, _ := orders.GetOrder(ctx, placed.ID)
	if expired.Status != repository.OrderStatusCancelled ||
		expired.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("setup: sweep must expire the order, got status=%v reason=%d", expired.Status, expired.CancelReason)
	}

	promo := &flakyCommitter{}
	c := consumer.NewPaymentConsumer(orders, nil, nil, consumer.WithVoucherCommitter(promo))
	env := envelopeFor(t, "evt_late_expiry", paid(placed.ID))
	for i := 0; i < 2; i++ { // first delivery, then a redelivery
		if err := c.HandleEnvelope(ctx, env); err != nil {
			t.Fatalf("delivery %d: a late payment must be acknowledged, got %v", i+1, err)
		}
	}

	got, _ := orders.GetOrder(ctx, placed.ID)
	if got.Status != repository.OrderStatusCancelled || got.PaidAt != nil ||
		got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("order must stay Cancelled/unpaid with its expiry reason: %+v", got)
	}
	if promo.attempts != 0 {
		t.Fatalf("an expired order's voucher must not be committed (%d attempts)", promo.attempts)
	}
	for _, row := range outbox.EnqueuedRows() {
		if row.EventType == events.OrderPaidEventType {
			t.Fatalf("a late settle must not emit OrderPaid: %+v", row)
		}
	}
}
