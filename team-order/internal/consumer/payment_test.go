package consumer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/repository"
)

// countingOrderStore records how many times UpdateOrderStatus is called so the
// test can prove the PAID transition happens exactly once.
type countingOrderStore struct {
	mu      sync.Mutex
	orders  map[string]repository.Order
	updates int
}

func (s *countingOrderStore) GetOrder(_ context.Context, id string) (repository.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return repository.Order{}, repository.ErrOrderNotFound
	}
	return o, nil
}

func (s *countingOrderStore) UpdateOrderStatus(_ context.Context, id string, st repository.OrderStatus, _ string) (repository.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return repository.Order{}, repository.ErrOrderNotFound
	}
	o.Status = st
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
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", Status: repository.OrderStatusPending},
	}}
	dedupe := repository.NewInMemoryProcessedEventRepository()
	c := consumer.NewPaymentConsumer(store, dedupe, nil)

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

	if store.updates != 1 {
		t.Fatalf("expected exactly 1 PAID transition, got %d", store.updates)
	}
	o, _ := store.GetOrder(ctx, "ord_1")
	if o.Status != repository.OrderStatusPaid {
		t.Fatalf("expected order PAID, got status %d", o.Status)
	}
}

// A second, distinct redelivery (different event_id but same order already PAID)
// is still a no-op because the transition is guarded on PENDING.
func TestPaymentConsumer_AlreadyPaidOrder_NoDoubleTransition(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", Status: repository.OrderStatusPaid},
	}}
	c := consumer.NewPaymentConsumer(store, nil, nil)

	env := envelopeFor(t, "evt_late", &paymentv1.PaymentSettled{
		OrderId: "ord_1",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if store.updates != 0 {
		t.Fatalf("expected no transition for an already-PAID order, got %d", store.updates)
	}
}

// FAILED settlements never drive the order to PAID.
func TestPaymentConsumer_FailedPayment_NoTransition(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", Status: repository.OrderStatusPending},
	}}
	c := consumer.NewPaymentConsumer(store, nil, nil)

	env := envelopeFor(t, "evt_fail", &paymentv1.PaymentSettled{
		OrderId: "ord_1",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if store.updates != 0 {
		t.Fatalf("expected no PAID transition for a FAILED payment, got %d", store.updates)
	}
}

// ── AD1: the Run loop commits only after apply/DLQ, and DLQs poison records ──

type fakeReader struct {
	records   []consumer.Record
	idx       int
	committed []consumer.Record
}

func (r *fakeReader) Fetch(ctx context.Context) (consumer.Record, error) {
	if r.idx >= len(r.records) {
		<-ctx.Done() // block until cancelled once records are drained
		return consumer.Record{}, ctx.Err()
	}
	rec := r.records[r.idx]
	r.idx++
	return rec, nil
}

func (r *fakeReader) Commit(_ context.Context, rec consumer.Record) error {
	r.committed = append(r.committed, rec)
	return nil
}

type fakeDLQ struct {
	produced []consumer.Record
}

func (d *fakeDLQ) Produce(_ context.Context, _ string, rec consumer.Record) error {
	d.produced = append(d.produced, rec)
	return nil
}

func TestPaymentConsumer_Run_PoisonRecordGoesToDLQ_ThenCommits(t *testing.T) {
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", Status: repository.OrderStatusPending},
	}}
	c := consumer.NewPaymentConsumer(store, nil, nil)

	good := envelopeFor(t, "evt_ok", &paymentv1.PaymentSettled{OrderId: "ord_1", Status: paymentv1.PaymentStatus_PAYMENT_STATUS_PAID})
	goodBytes, _ := proto.Marshal(good)

	reader := &fakeReader{records: []consumer.Record{
		{Key: "ord_x", Value: []byte("not a valid envelope")}, // poison → DLQ
		{Key: "ord_1", Value: goodBytes},                      // applied
	}}
	dlq := &fakeDLQ{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = c.Run(ctx, reader, dlq, consumer.RunConfig{MaxAttempts: 2, BaseBackoff: 1})
		close(done)
	}()

	// Wait until both records have been committed, then stop the loop.
	waitFor(t, func() bool { return len(reader.committed) == 2 })
	cancel()
	<-done

	if len(dlq.produced) != 1 {
		t.Fatalf("expected 1 poison record in DLQ, got %d", len(dlq.produced))
	}
	if store.updates != 1 {
		t.Fatalf("expected the good record applied once, got %d", store.updates)
	}
	if len(reader.committed) != 2 {
		t.Fatalf("expected both records committed after handling, got %d", len(reader.committed))
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
