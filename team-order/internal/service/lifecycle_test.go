package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
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

// ---- task 6: expiry pass ----

func (r *lifecycleRig) status(t *testing.T, id string) repository.Order {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r *lifecycleRig) cancelled(t *testing.T) int {
	t.Helper()
	os, err := r.orders.ListBuyerOrders(context.Background(), "buyer_1", int32(repository.OrderStatusCancelled))
	if err != nil {
		t.Fatal(err)
	}
	return len(os)
}

// expiryRig is a real checkout (stock, voucher hold, committed flash line) of one
// MOCK_MOMO order with a 1h payment deadline.
type expiryRig struct {
	domain *pricedDomain
	orders *repository.InMemoryOrderRepository
	outbox *repository.InMemoryOutboxRepository
	sagas  *repository.InMemorySagaRepository
	promo  *fakePromotion
	flash  *holdFake
	svc    *service.OrderService
	order  repository.Order
}

func newExpiryRig(t *testing.T, voucher string) *expiryRig {
	t.Helper()
	r := &expiryRig{
		outbox: repository.NewInMemoryOutboxRepository(),
		sagas:  repository.NewInMemorySagaRepository(),
		promo:  newFakePromotion(500),
		flash:  &holdFake{sales: map[string]*promotionv1.GetActiveFlashSaleResponse{"lst_1": activeSale("", 350000)}},
		domain: &pricedDomain{
			Domain:   upstreamtest.NewDomain(map[string]int32{"lst_1": 10}),
			listings: map[string]*listingv1.Listing{"lst_1": {Id: "lst_1", Price: 500000, SellerId: "s1", Stock: 10}},
		},
	}
	r.orders = repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow), repository.WithInMemoryOutbox(r.outbox))
	carts := repository.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), repository.CartItem{UserID: "buyer_1", ListingID: "lst_1",
		Quantity: 2, UnitPrice: 350000, SellerID: "s1", Title: "lst_1"}); err != nil {
		t.Fatal(err)
	}
	r.svc = service.NewOrderService(r.orders, carts, nil, nil, r.domain, nil, nil,
		service.WithSagaRepository(r.sagas), service.WithPromotionClient(r.promo),
		service.WithOrderFlashSaleClient(r.flash), service.WithPaymentDeadline(time.Hour),
		service.WithPaymentReminderBefore(lifecycleOffset), service.WithReleaseRetry(time.Second, 2, time.Millisecond))
	placed, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil,
		int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), voucher)
	if err != nil || len(placed) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	r.order = placed[0]
	if r.order.PaymentDueAt == nil {
		t.Fatal("setup: an online-pay order must carry a payment deadline")
	}
	if r.domain.Stock("lst_1") != 8 {
		t.Fatalf("setup stock %d", r.domain.Stock("lst_1"))
	}
	return r
}

func (r *expiryRig) due() time.Time { return *r.order.PaymentDueAt }

// Scenario: Order expiry: Order unpaid past its deadline is cancelled and its stock released
func TestExpiryCancelsUnpaidOrderAndReleasesStock(t *testing.T) {
	r := newExpiryRig(t, "")
	r.svc.SweepLifecycle(context.Background(), r.due().Add(time.Second))
	got, _ := r.orders.GetOrder(context.Background(), r.order.ID)
	if got.Status != repository.OrderStatusCancelled ||
		got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("want Cancelled/PAYMENT_EXPIRED, got status=%v reason=%d", got.Status, got.CancelReason)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10 (released)", r.domain.Stock("lst_1"))
	}
}

// Scenario: Order expiry: Expiry emits OrderCancelled with the expiry reason
func TestExpiryEmitsOrderCancelledWithExpiryReason(t *testing.T) {
	r := newExpiryRig(t, "")
	r.svc.SweepLifecycle(context.Background(), r.due().Add(time.Second))
	r.svc.SweepLifecycle(context.Background(), r.due().Add(2*time.Second))
	facts := cancelFacts(t, r.outbox, r.order.ID)
	if len(facts) != 1 {
		t.Fatalf("want exactly 1 OrderCancelled, got %d", len(facts))
	}
	if f := facts[0]; f.GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PENDING ||
		f.GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED || f.GetBuyerId() != "buyer_1" {
		t.Fatalf("fact: %+v", f)
	}
}

// Scenario: Order expiry: Expiry releases the voucher hold and flash units once
func TestExpiryReleasesVoucherAndFlashOnce(t *testing.T) {
	r := newExpiryRig(t, "VOUCHER")
	hold := service.FlashReservationID(r.order.ID, "lst_1", "")
	r.svc.SweepLifecycle(context.Background(), r.due().Add(time.Second))
	if len(r.promo.releasedIDs) != 1 || r.promo.releasedIDs[0] != r.order.ID {
		t.Fatalf("voucher hold must be released once under the order id: %v", r.promo.releasedIDs)
	}
	if got := r.flash.releasedIDs(); len(got) != 1 || got[0] != hold {
		t.Fatalf("flash hold must be released once: %v, want [%s]", got, hold)
	}
	releases := r.domain.Calls.Release
	if releases != 1 {
		t.Fatalf("domain release calls %d, want 1", releases)
	}
	r.svc.SweepLifecycle(context.Background(), r.due().Add(time.Minute))
	if len(r.promo.releasedIDs) != 1 || len(r.flash.releasedIDs()) != 1 || r.domain.Calls.Release != releases {
		t.Fatalf("second sweep must release nothing: voucher=%v flash=%v domain=%d",
			r.promo.releasedIDs, r.flash.releasedIDs(), r.domain.Calls.Release)
	}
}

// Scenario: Order expiry: Boundary at the deadline keeps the order payable
func TestExpiryBoundaryStrictlyAfterDeadline(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	r.svc.SweepLifecycle(context.Background(), due)
	if got := r.status(t, o.ID); got.Status != repository.OrderStatusPending {
		t.Fatalf("at exactly the deadline the order stays payable, got %v", got.Status)
	}
	r.svc.SweepLifecycle(context.Background(), due.Add(time.Second))
	if got := r.status(t, o.ID); got.Status != repository.OrderStatusCancelled {
		t.Fatalf("one second after the deadline: want Cancelled, got %v", got.Status)
	}
}

// Scenario: Order expiry: A COD order is never expired
func TestExpirySkipsCodOrder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	cod := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, nil)
	codWithDue := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, &due)
	control := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due) // positive control
	r.svc.SweepLifecycle(context.Background(), due.Add(48*time.Hour))
	for _, id := range []string{cod.ID, codWithDue.ID} {
		if got := r.status(t, id); got.Status != repository.OrderStatusPending {
			t.Fatalf("COD order %s must stay Pending, got %v", id, got.Status)
		}
		if n := len(cancelFacts(t, r.outbox, id)); n != 0 {
			t.Fatalf("COD order %s: want no OrderCancelled, got %d", id, n)
		}
	}
	if got := r.status(t, control.ID); got.Status != repository.OrderStatusCancelled {
		t.Fatalf("the online control order must expire, got %v", got.Status)
	}
}

// Scenario: Order expiry: An order without a deadline is never expired
func TestExpirySkipsOrderWithoutDeadline(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	legacy := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, nil)
	control := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	r.svc.SweepLifecycle(context.Background(), due.Add(1000*time.Hour))
	if got := r.status(t, legacy.ID); got.Status != repository.OrderStatusPending {
		t.Fatalf("an order with no deadline stays Pending, got %v", got.Status)
	}
	if got := r.status(t, control.ID); got.Status != repository.OrderStatusCancelled {
		t.Fatalf("control must expire, got %v", got.Status)
	}
}

// Scenario: Order expiry: A paid order is never expired
func TestExpirySkipsPaidOrder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	paid := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	control := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), paid.ID, repository.OrderStatusPaid,
		[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	r.svc.SweepLifecycle(context.Background(), due.Add(time.Hour))
	if got := r.status(t, paid.ID); got.Status != repository.OrderStatusPaid {
		t.Fatalf("a paid order stays Paid, got %v", got.Status)
	}
	if n := len(cancelFacts(t, r.outbox, paid.ID)); n != 0 {
		t.Fatalf("paid order: want no OrderCancelled, got %d", n)
	}
	if got := r.status(t, control.ID); got.Status != repository.OrderStatusCancelled {
		t.Fatalf("control must expire, got %v", got.Status)
	}
}

// Scenario: Order expiry: Two sweeps expire an order once
func TestExpiryConcurrentSweepsReleaseOnce(t *testing.T) {
	r := newExpiryRig(t, "VOUCHER")
	now := r.due().Add(time.Second)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; r.svc.SweepLifecycle(context.Background(), now) }()
	}
	close(start)
	wg.Wait()
	if got := r.status2(t); got.Status != repository.OrderStatusCancelled {
		t.Fatalf("want Cancelled, got %v", got.Status)
	}
	if n := len(cancelFacts(t, r.outbox, r.order.ID)); n != 1 {
		t.Fatalf("want exactly 1 OrderCancelled, got %d", n)
	}
	if r.domain.Calls.Release != 1 || len(r.promo.releasedIDs) != 1 || len(r.flash.releasedIDs()) != 1 {
		t.Fatalf("each hold released exactly once: domain=%d voucher=%v flash=%v",
			r.domain.Calls.Release, r.promo.releasedIDs, r.flash.releasedIDs())
	}
}

func (r *expiryRig) status2(t *testing.T) repository.Order {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), r.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// Scenario: Order expiry: A failed release leaves the order cancelled and parked
func TestExpiryReleaseFailureParksForSweep(t *testing.T) {
	r := newExpiryRig(t, "")
	r.domain.ReleaseErr = func(string) error { return status.Error(codes.Unavailable, "team-domain stopped") }
	r.svc.SweepLifecycle(context.Background(), r.due().Add(time.Second))
	if got := r.status2(t); got.Status != repository.OrderStatusCancelled ||
		got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("order must be Cancelled/PAYMENT_EXPIRED despite the failed release: %+v", got)
	}
	held, _ := r.sagas.ListReservationsByOrder(context.Background(), r.order.ID)
	if len(held) != 1 || held[0].Status != repository.ReservationStatusReleaseFailed {
		t.Fatalf("release must be parked RELEASE_FAILED: %+v", held)
	}
	r.domain.ReleaseErr = nil
	if n, err := r.svc.SweepExpiredReservations(context.Background(), time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("reservation sweep: %d %v", n, err)
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock %d, want 10", r.domain.Stock("lst_1"))
	}
}

// failingExpiryRepo makes the expiry query fail, as a database error would.
type failingExpiryRepo struct {
	*repository.InMemoryOrderRepository
	calls int
}

func (f *failingExpiryRepo) FindExpired(context.Context, time.Time, int) ([]repository.Order, error) {
	f.calls++
	return nil, errors.New("database unavailable")
}

// Scenario: Order expiry: One failing pass does not stop the next
func TestSweepLifecyclePassesAreIndependent(t *testing.T) {
	outbox := repository.NewInMemoryOutboxRepository()
	inner := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow), repository.WithInMemoryOutbox(outbox))
	repo := &failingExpiryRepo{InMemoryOrderRepository: inner}
	svc := service.NewOrderService(repo, repository.NewInMemoryCartRepository(), nil, nil,
		upstreamtest.NewDomain(map[string]int32{}), nil, nil, service.WithPaymentReminderBefore(lifecycleOffset))
	due := lifecycleDue
	o, err := inner.CreateOrder(context.Background(), repository.Order{BuyerID: "buyer_1", SellerID: "s1", TotalAmount: 3000,
		Currency: "VND", PaymentMethod: int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), PaymentDueAt: &due,
		Items: []repository.OrderItem{{ListingID: "lst_1", Quantity: 1, UnitPrice: 3000}}})
	if err != nil {
		t.Fatal(err)
	}
	svc.SweepLifecycle(context.Background(), due.Add(-time.Hour))
	if repo.calls == 0 {
		t.Fatal("the tick must run the expiry pass (it is the failing pass under test)")
	}
	n := 0
	for _, row := range outbox.EnqueuedRows() {
		if row.EventType == events.OrderPaymentReminderEventType && row.AggregateID == o.ID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the reminder pass must still run after a failing expiry pass: %d reminders", n)
	}
}

// Scenario: Order expiry: A sweep that first sees an order past its deadline sends no reminder
// (the cancel half; the no-reminder half is TestLateFirstSweepExpiresWithoutReminder).
func TestLateFirstSweepCancelsOrder(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	o := r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &due)
	r.svc.SweepLifecycle(context.Background(), due.Add(time.Minute))
	got := r.status(t, o.ID)
	if got.Status != repository.OrderStatusCancelled ||
		got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
		t.Fatalf("a late first sweep must expire the order: %+v", got)
	}
}

// Scenario: Order expiry: An expiry pass handles at most 100 orders per tick
func TestExpiryPassBatchBoundPerTick(t *testing.T) {
	r := newLifecycleRig(t)
	due := lifecycleDue
	for i := 0; i < 250; i++ {
		d := due
		r.place(t, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, &d)
	}
	now := due.Add(time.Minute)
	for tick, want := range []int{100, 200, 250} {
		r.svc.SweepLifecycle(context.Background(), now)
		if got := r.cancelled(t); got != want {
			t.Fatalf("after tick %d: want %d cancelled, got %d", tick+1, want, got)
		}
	}
}
