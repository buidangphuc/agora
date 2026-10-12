package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

// countingOrderStore records how many status writes succeed so the
// test can prove the PAID transition happens exactly once.
type countingOrderStore struct {
	mu      sync.Mutex
	orders  map[string]store.Order
	updates int
}

func (s *countingOrderStore) GetOrder(_ context.Context, id string) (store.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	return o, nil
}

func (s *countingOrderStore) UpdateOrderStatusFrom(_ context.Context, id string, to store.OrderStatus, from []store.OrderStatus, trackingNumber string) (store.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	allowed := false
	for _, st := range from {
		if o.Status == st {
			allowed = true
		}
	}
	if !allowed {
		return store.Order{}, store.ErrStatusConflict
	}
	o.Status = to
	if trackingNumber != "" {
		o.TrackingNumber = trackingNumber
	}
	if to == store.OrderStatusPaid {
		now := time.Now()
		o.PaidAt = &now
	}
	s.orders[id] = o
	s.updates++
	return o, nil
}

func envelopeFor(t *testing.T, eventID string, settled *paymentv1.PaymentSettled) *eventsv1.EventEnvelope {
	t.Helper()
	payload, err := proto.Marshal(settled)
	if err != nil {
		t.Fatalf("marshal settled: %v", err)
	}
	return &eventsv1.EventEnvelope{
		EventId: eventID,
		Type:    "platform.payment.v1.PaymentSettled",
		Payload: payload,
	}
}

// AD4/H3: consuming the same PaymentSettled twice transitions the order once.
func TestPaymentConsumer_IdempotentDoubleConsume(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_1": {ID: "ord_1", Status: store.OrderStatusPending},
	}}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil)

	env := envelopeFor(t, "evt_1", &paymentv1.PaymentSettled{
		PaymentId: "pay_1",
		OrderId:   "ord_1",
		Status:    paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})

	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("first consume failed: %v", err)
	}
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("second (duplicate) consume failed: %v", err)
	}

	if fake.updates != 1 {
		t.Fatalf("expected exactly 1 PAID transition, got %d", fake.updates)
	}
	o, _ := fake.GetOrder(ctx, "ord_1")
	if o.Status != store.OrderStatusPaid {
		t.Fatalf("expected order PAID, got status %d", o.Status)
	}
}

// A second, distinct redelivery (different event_id but same order already PAID)
// is still a no-op because the transition is guarded on PENDING.
func TestPaymentConsumer_AlreadyPaidOrder_NoDoubleTransition(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_1": {ID: "ord_1", Status: store.OrderStatusPaid},
	}}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil)

	env := envelopeFor(t, "evt_late", &paymentv1.PaymentSettled{
		OrderId: "ord_1",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if fake.updates != 0 {
		t.Fatalf("expected no transition for an already-PAID order, got %d", fake.updates)
	}
}

// FAILED settlements never drive the order to PAID.
func TestPaymentConsumer_FailedPayment_NoTransition(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_1": {ID: "ord_1", Status: store.OrderStatusPending},
	}}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil)

	env := envelopeFor(t, "evt_fail", &paymentv1.PaymentSettled{
		OrderId: "ord_1",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if fake.updates != 0 {
		t.Fatalf("expected no PAID transition for a FAILED payment, got %d", fake.updates)
	}
}
