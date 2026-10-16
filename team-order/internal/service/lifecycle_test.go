package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

const lifecycleOffset = 2 * time.Hour

var lifecycleDue = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

type lifecycleRig struct {
	orders *repository.InMemoryOrderRepository
	outbox *repository.InMemoryOutboxRepository
	svc    *service.OrderService
}

func newLifecycleRig(t *testing.T) *lifecycleRig {
	t.Helper()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow), repository.WithInMemoryOutbox(outbox))
	svc := service.NewOrderService(orders, repository.NewInMemoryCartRepository(), nil, nil,
		upstreamtest.NewDomain(map[string]int32{}), nil, nil,
		service.WithPaymentReminderBefore(lifecycleOffset))
	return &lifecycleRig{orders: orders, outbox: outbox, svc: svc}
}

func (r *lifecycleRig) place(t *testing.T, method paymentv1.PaymentMethod, due *time.Time) repository.Order {
	t.Helper()
	o, err := r.orders.CreateOrder(context.Background(), repository.Order{BuyerID: "buyer_1", SellerID: "s1",
		TotalAmount: 3000, Currency: "VND", PaymentMethod: int32(method), PaymentDueAt: due,
		Items: []repository.OrderItem{{ListingID: "lst_1", Quantity: 1, UnitPrice: 3000}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r *lifecycleRig) reminders(t *testing.T, orderID string) []*orderv1.OrderPaymentReminder {
	t.Helper()
	var out []*orderv1.OrderPaymentReminder
	for _, row := range r.outbox.EnqueuedRows() {
		if row.EventType != events.OrderPaymentReminderEventType || row.AggregateID != orderID {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderPaymentReminder
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func (r *lifecycleRig) allReminders() int {
	n := 0
	for _, row := range r.outbox.EnqueuedRows() {
		if row.EventType == events.OrderPaymentReminderEventType {
			n++
		}
	}
	return n
}

// Scenario: Order expiry: Reminder is emitted two hours before the deadline
func TestReminderEmittedInsideWindow(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	r.svc.SweepLifecycle(context.Background(), due.Add(-lifecycleOffset).Add(time.Minute))
	got := r.reminders(t, o.ID)
	if len(got) != 1 || !got[0].GetPaymentDueAt().AsTime().Equal(due) || got[0].GetBuyerId() != "buyer_1" ||
		got[0].GetTotalAmount() != 3000 || got[0].GetCurrency() != "VND" {
		t.Fatalf("want one reminder carrying the order's deadline, got %+v", got)
	}
	stored, _ := r.orders.GetOrder(context.Background(), o.ID)
	if stored.PaymentReminderSentAt == nil || stored.Status != repository.OrderStatusPending {
		t.Fatalf("stamped and still pending expected: %+v", stored)
	}
}

// Scenario: Order expiry: Reminder boundary is inclusive at deadline minus offset
func TestReminderBoundary(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	start := due.Add(-lifecycleOffset)
	r.svc.SweepLifecycle(context.Background(), start.Add(-time.Second))
	if n := len(r.reminders(t, o.ID)); n != 0 {
		t.Fatalf("one second early: want 0 reminders, got %d", n)
	}
	r.svc.SweepLifecycle(context.Background(), start)
	if n := len(r.reminders(t, o.ID)); n != 1 {
		t.Fatalf("at exactly deadline-minus-offset: want 1 reminder, got %d", n)
	}
}

// Scenario: Order expiry: Repeated sweeps never repeat the reminder
func TestReminderEmittedOnce(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	now := due.Add(-time.Hour)
	for i := 0; i < 3; i++ {
		r.svc.SweepLifecycle(context.Background(), now.Add(time.Duration(i)*time.Minute))
	}
	if n := len(r.reminders(t, o.ID)); n != 1 {
		t.Fatalf("three sweeps: want 1 reminder, got %d", n)
	}

	// Two replicas sweeping concurrently over a fresh order.
	r2 := newLifecycleRig(t)
	o2 := r2.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r2.svc.SweepLifecycle(context.Background(), now) }()
	}
	wg.Wait()
	if n := len(r2.reminders(t, o2.ID)); n != 1 {
		t.Fatalf("two concurrent replicas: want 1 reminder, got %d", n)
	}
}

// Scenario: Order expiry: A paid order is never reminded
func TestReminderSkipsPaidOrder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), o.ID, repository.OrderStatusPaid,
		[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	r.svc.SweepLifecycle(context.Background(), due.Add(-time.Hour))
	if n := len(r.reminders(t, o.ID)); n != 0 {
		t.Fatalf("paid order: want 0 reminders, got %d", n)
	}
}

// Scenario: Order expiry: A COD order is never reminded
func TestReminderSkipsCodOrder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	cod := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, nil)
	// Even a COD row that somehow carries a deadline is never reminded.
	codWithDue := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, &due)
	r.svc.SweepLifecycle(context.Background(), due.Add(-time.Hour))
	if n := len(r.reminders(t, cod.ID)) + len(r.reminders(t, codWithDue.ID)); n != 0 {
		t.Fatalf("COD orders: want 0 reminders, got %d", n)
	}
}

// Scenario: Order expiry: A sweep that first sees an order past its deadline sends no reminder
// (the cancel itself is asserted by the expiry-pass task; this pins the no-reminder half).
func TestLateFirstSweepExpiresWithoutReminder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	r.svc.SweepLifecycle(context.Background(), due.Add(time.Minute))
	if n := len(r.reminders(t, o.ID)); n != 0 {
		t.Fatalf("late first sweep: want 0 reminders, got %d", n)
	}
	if stored, _ := r.orders.GetOrder(context.Background(), o.ID); stored.PaymentReminderSentAt != nil {
		t.Fatal("a late order must not be stamped as reminded")
	}
}

// Scenario: Order expiry: A reminder pass handles at most 100 orders per tick
func TestReminderPassBatchBoundPerTick(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	for i := 0; i < 250; i++ {
		d := due
		r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &d)
	}
	now := due.Add(-time.Hour)
	for tick, want := range []int{100, 200, 250} {
		r.svc.SweepLifecycle(context.Background(), now)
		if got := r.allReminders(); got != want {
			t.Fatalf("after tick %d: want %d reminder rows, got %d", tick+1, want, got)
		}
	}
}
