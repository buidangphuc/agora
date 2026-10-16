package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

const (
	rcAutoComplete = 7 * 24 * time.Hour
	rcAutoApprove  = 3 * 24 * time.Hour
	rcBatch        = 100
)

type rcRig struct {
	svc     *service.OrderService
	orders  *repository.InMemoryOrderRepository
	returns *repository.InMemoryReturnRepository
	outbox  *repository.InMemoryOutboxRepository
}

func newRCRig() *rcRig {
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository(
		repository.WithCompletedOutbox(events.BuildCompletedOutboxRow),
		repository.WithInMemoryOutbox(outbox),
		repository.WithAutoCompleteAfter(rcAutoComplete),
		repository.WithReturnWindow(15*24*time.Hour),
		repository.WithShipByAfter(2*24*time.Hour))
	returns := repository.NewInMemoryReturnRepository(
		repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders),
		repository.WithReturnAutoApproveAfter(rcAutoApprove))
	orders.SetReturnSource(returns)
	return &rcRig{svc: service.NewOrderService(orders, nil, returns, nil, nil, nil, nil), orders: orders, returns: returns, outbox: outbox}
}

// shipped creates a SHIPPED order (paid online when paid, else cash on delivery).
func (r *rcRig) shipped(t *testing.T, paid bool) repository.Order {
	t.Helper()
	ctx := context.Background()
	o, err := r.orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 5000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	from := repository.OrderStatusPending
	if paid {
		if _, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, []repository.OrderStatus{from}, ""); err != nil {
			t.Fatal(err)
		}
		from = repository.OrderStatusPaid
	}
	o, err = r.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{from}, "T")
	if err != nil {
		t.Fatal(err)
	}
	if o.AutoCompleteAt == nil {
		t.Fatal("setup: shipping must stamp auto_complete_at")
	}
	return o
}

func (r *rcRig) newReturn(t *testing.T, orderID string, st repository.ReturnStatus) repository.OrderReturn {
	t.Helper()
	ctx := context.Background()
	ret, err := r.returns.CreateReturn(ctx, repository.OrderReturn{OrderID: orderID, BuyerID: "buyer_1", SellerID: "seller_1", Reason: "x", RefundAmount: 1000, Status: repository.ReturnStatusPending})
	if err != nil {
		t.Fatal(err)
	}
	path := map[repository.ReturnStatus][]repository.ReturnStatus{
		repository.ReturnStatusPending:  {},
		repository.ReturnStatusApproved: {repository.ReturnStatusApproved},
		repository.ReturnStatusRejected: {repository.ReturnStatusRejected},
		repository.ReturnStatusRefunded: {repository.ReturnStatusApproved, repository.ReturnStatusRefunded},
	}[st]
	cur := repository.ReturnStatusPending
	for _, next := range path {
		if ret, err = r.returns.TransitionReturn(ctx, ret.ID, cur, next); err != nil {
			t.Fatal(err)
		}
		cur = next
	}
	return ret
}

func (r *rcRig) order(t *testing.T, id string) repository.Order {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r *rcRig) ret(t *testing.T, id string) repository.OrderReturn {
	t.Helper()
	o, err := r.returns.GetReturn(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func (r *rcRig) completedFacts(t *testing.T, orderID string) []*orderv1.OrderCompleted {
	t.Helper()
	var out []*orderv1.OrderCompleted
	for _, row := range r.outbox.EnqueuedRows() {
		if row.AggregateID != orderID || row.EventType != events.OrderCompletedEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderCompleted
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

// pastDeadline is a sweep time after every default deadline; beforeDeadline is
// the present (every stamped deadline lies in the future).
func pastDeadline() time.Time   { return time.Now().Add(30 * 24 * time.Hour) }
func beforeDeadline() time.Time { return time.Now() }

func (r *rcRig) sweepComplete(t *testing.T, now time.Time) int {
	t.Helper()
	n, err := r.svc.AutoCompleteShipped(context.Background(), now)
	if err != nil {
		t.Fatalf("AutoCompleteShipped: %v", err)
	}
	return n
}

func (r *rcRig) sweepApprove(t *testing.T, now time.Time) int {
	t.Helper()
	n, err := r.svc.AutoApproveReturns(context.Background(), now)
	if err != nil {
		t.Fatalf("AutoApproveReturns: %v", err)
	}
	return n
}

// Scenario: Order receipt: Unconfirmed shipped order completes after the window
func TestSweepCompletesUnconfirmedShippedOrder(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	if n := r.sweepComplete(t, pastDeadline()); n != 1 {
		t.Fatalf("completed %d orders, want 1", n)
	}
	got := r.order(t, o.ID)
	if got.Status != repository.OrderStatusCompleted || got.CompletedAt == nil || got.AutoCompleteAt != nil {
		t.Fatalf("want COMPLETED with completed_at and no auto_complete_at: %+v", got)
	}
	facts := r.completedFacts(t, o.ID)
	if len(facts) != 1 || facts[0].GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_SYSTEM {
		t.Fatalf("want exactly one SYSTEM OrderCompleted, got %v", facts)
	}
}

// Scenario: Order receipt: A shipped order inside its window is not completed
func TestSweepDoesNotCompleteBeforeDeadline(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	if n := r.sweepComplete(t, beforeDeadline()); n != 0 {
		t.Fatalf("completed %d orders, want 0", n)
	}
	if r.order(t, o.ID).Status != repository.OrderStatusShipped || len(r.completedFacts(t, o.ID)) != 0 {
		t.Fatal("order must stay SHIPPED with no OrderCompleted")
	}
}

// Scenario: Order receipt: An open return blocks auto-complete
func TestSweepSkipsOrderWithOpenReturn(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusPending)
	now := pastDeadline()
	// Auto-approve is not part of this pass: the return stays PENDING and blocks.
	if n := r.sweepComplete(t, now); n != 0 {
		t.Fatalf("completed %d orders with a PENDING return, want 0", n)
	}
	if r.order(t, o.ID).Status != repository.OrderStatusShipped {
		t.Fatal("order must stay SHIPPED")
	}
	if _, err := r.returns.TransitionReturn(context.Background(), ret.ID, repository.ReturnStatusPending, repository.ReturnStatusRejected); err != nil {
		t.Fatal(err)
	}
	if n := r.sweepComplete(t, now); n != 1 || r.order(t, o.ID).Status != repository.OrderStatusCompleted {
		t.Fatalf("first run after REJECTED must complete the order (n=%d)", n)
	}
}

// Scenario: Order receipt: An approved unrefunded return blocks auto-complete
func TestSweepSkipsOrderWithApprovedReturn(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusApproved)
	now := pastDeadline()
	if n := r.sweepComplete(t, now); n != 0 || r.order(t, o.ID).Status != repository.OrderStatusShipped {
		t.Fatalf("an APPROVED return on an online-paid order must block (n=%d)", n)
	}
	if _, err := r.returns.TransitionReturn(context.Background(), ret.ID, repository.ReturnStatusApproved, repository.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}
	if n := r.sweepComplete(t, now); n != 1 || r.order(t, o.ID).Status != repository.OrderStatusCompleted {
		t.Fatalf("first run after REFUNDED must complete the order (n=%d)", n)
	}
}

// Scenario: Order receipt: An approved return on a cash-on-delivery order does not block auto-complete
func TestSweepCompletesCODOrderWithApprovedReturn(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, false)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusApproved)
	if n := r.sweepComplete(t, pastDeadline()); n != 1 {
		t.Fatalf("completed %d orders, want 1", n)
	}
	if r.order(t, o.ID).Status != repository.OrderStatusCompleted {
		t.Fatal("COD order must complete")
	}
	if f := r.completedFacts(t, o.ID); len(f) != 1 || f[0].GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_SYSTEM || f[0].GetPaidOnline() {
		t.Fatalf("want one SYSTEM, not-paid-online OrderCompleted, got %v", f)
	}
	if r.ret(t, ret.ID).Status != repository.ReturnStatusApproved {
		t.Fatal("the return must still read APPROVED")
	}
}

// Scenario: Order receipt: A rejected return does not block auto-complete
func TestSweepCompletesOrderWithRejectedReturn(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	r.newReturn(t, o.ID, repository.ReturnStatusRejected)
	if n := r.sweepComplete(t, pastDeadline()); n != 1 || r.order(t, o.ID).Status != repository.OrderStatusCompleted {
		t.Fatalf("a REJECTED return must not block (n=%d)", n)
	}
}

// Scenario: Order receipt: Confirm racing the sweep completes once
func TestConfirmReceiptRacesSweepOneCompletion(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	now := pastDeadline()
	var wg sync.WaitGroup
	var confirmErr, sweepErr error
	var swept int
	wg.Add(2)
	go func() { defer wg.Done(); _, confirmErr = r.svc.ConfirmReceipt(context.Background(), o.ID) }()
	go func() { defer wg.Done(); swept, sweepErr = r.svc.AutoCompleteShipped(context.Background(), now) }()
	wg.Wait()
	if sweepErr != nil {
		t.Fatalf("a lost race must be skipped, not fail the sweep: %v", sweepErr)
	}
	if (confirmErr == nil) == (swept == 1) {
		t.Fatalf("exactly one side must win: confirmErr=%v swept=%d", confirmErr, swept)
	}
	if r.order(t, o.ID).Status != repository.OrderStatusCompleted {
		t.Fatal("order must be COMPLETED")
	}
	if f := r.completedFacts(t, o.ID); len(f) != 1 {
		t.Fatalf("want exactly one OrderCompleted, got %d", len(f))
	}
}

// Scenario: Order receipt: The sweep handles at most one batch per run
func TestSweepAutoCompleteBatchBound(t *testing.T) {
	r := newRCRig()
	ids := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		ids = append(ids, r.shipped(t, true).ID)
	}
	now := pastDeadline()
	completed := func() (done, open []repository.Order) {
		for _, id := range ids {
			o := r.order(t, id)
			if o.Status == repository.OrderStatusCompleted {
				done = append(done, o)
			} else {
				open = append(open, o)
			}
		}
		return
	}
	if n := r.sweepComplete(t, now); n != rcBatch {
		t.Fatalf("first run completed %d, want %d", n, rcBatch)
	}
	done, open := completed()
	if len(done) != rcBatch || len(open) != 150 {
		t.Fatalf("after run 1: %d completed, %d open", len(done), len(open))
	}
	// Oldest deadline first: no completed order has a later deadline than an open one.
	var latestDone, earliestOpen time.Time
	for _, o := range done {
		if o.ShippedAt.After(latestDone) {
			latestDone = *o.ShippedAt
		}
	}
	earliestOpen = *open[0].ShippedAt
	for _, o := range open {
		if o.ShippedAt.Before(earliestOpen) {
			earliestOpen = *o.ShippedAt
		}
	}
	if latestDone.After(earliestOpen) {
		t.Fatalf("not oldest-first: completed up to %v, open from %v", latestDone, earliestOpen)
	}
	if n := r.sweepComplete(t, now); n != rcBatch {
		t.Fatalf("second run completed %d, want %d", n, rcBatch)
	}
	if n := r.sweepComplete(t, now); n != 50 {
		t.Fatalf("third run completed %d, want 50", n)
	}
	if _, open := completed(); len(open) != 0 {
		t.Fatalf("%d orders still open after three runs", len(open))
	}
}

// Scenario: Order receipt: A repeated sweep changes nothing
func TestSweepAutoCompleteIdempotent(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	now := pastDeadline()
	if n := r.sweepComplete(t, now); n != 1 {
		t.Fatalf("first run completed %d, want 1", n)
	}
	rows := len(r.outbox.EnqueuedRows())
	before := r.order(t, o.ID)
	if n := r.sweepComplete(t, now); n != 0 {
		t.Fatalf("second run completed %d, want 0", n)
	}
	if len(r.outbox.EnqueuedRows()) != rows || !r.order(t, o.ID).UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("the second run must write nothing")
	}
}

// Scenario: Order receipt: Unanswered return is approved after the window
func TestAutoApproveUnansweredReturn(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusPending)
	if n := r.sweepApprove(t, pastDeadline()); n != 1 {
		t.Fatalf("approved %d returns, want 1", n)
	}
	if r.ret(t, ret.ID).Status != repository.ReturnStatusApproved {
		t.Fatal("return must read APPROVED")
	}
	for _, row := range r.outbox.EnqueuedRows() {
		if row.EventType == events.ReturnRefundedEventType {
			t.Fatal("auto-approve must not write ReturnRefunded")
		}
	}
}

// Scenario: Order receipt: A return inside its answer window is not approved
func TestAutoApproveBeforeDeadlineNoOp(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusPending)
	if n := r.sweepApprove(t, beforeDeadline()); n != 0 {
		t.Fatalf("approved %d returns, want 0", n)
	}
	if r.ret(t, ret.ID).Status != repository.ReturnStatusPending {
		t.Fatal("return must stay PENDING")
	}
}

// Scenario: Order receipt: Rejected return is never auto-approved
func TestAutoApproveSkipsRejected(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusRejected)
	if n := r.sweepApprove(t, pastDeadline()); n != 0 || r.ret(t, ret.ID).Status != repository.ReturnStatusRejected {
		t.Fatalf("a REJECTED return must stay REJECTED (n=%d)", n)
	}
}

// Scenario: Order receipt: Approved return is not touched by the sweep
func TestAutoApproveSkipsApproved(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusApproved)
	before := r.ret(t, ret.ID)
	if n := r.sweepApprove(t, pastDeadline()); n != 0 {
		t.Fatalf("sweep reported %d approvals, want 0", n)
	}
	after := r.ret(t, ret.ID)
	if after.Status != repository.ReturnStatusApproved || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("approved return touched: %+v -> %+v", before, after)
	}
}

// Scenario: Order receipt: Seller answer racing the auto-approve produces one transition
func TestAutoApproveRacesSellerAnswerOneTransition(t *testing.T) {
	r := newRCRig()
	o := r.shipped(t, true)
	ret := r.newReturn(t, o.ID, repository.ReturnStatusPending)
	now := pastDeadline()
	var wg sync.WaitGroup
	var rejectErr, sweepErr error
	var swept int
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, rejectErr = r.svc.UpdateReturnStatus(context.Background(), ret.ID, repository.ReturnStatusRejected)
	}()
	go func() { defer wg.Done(); swept, sweepErr = r.svc.AutoApproveReturns(context.Background(), now) }()
	wg.Wait()
	if sweepErr != nil {
		t.Fatalf("a lost race must be skipped, not fail the sweep: %v", sweepErr)
	}
	got := r.ret(t, ret.ID).Status
	switch {
	case swept == 1 && rejectErr != nil:
		// the sweep won; the seller's late reject lost its compare-and-set
		if got != repository.ReturnStatusApproved {
			t.Fatalf("sweep won but return reads %v", got)
		}
	case swept == 0 && rejectErr == nil:
		if got != repository.ReturnStatusRejected {
			t.Fatalf("seller won but return reads %v", got)
		}
	case swept == 1 && rejectErr == nil:
		// reject applied after an approve is a legal seller move (APPROVED -> REJECTED);
		// the two are then sequential, not racing, and the final state is REJECTED.
		if got != repository.ReturnStatusRejected {
			t.Fatalf("approve then reject must end REJECTED, got %v", got)
		}
	default:
		t.Fatalf("neither side applied: swept=%d rejectErr=%v", swept, rejectErr)
	}
}

// Scenario: Order receipt: The auto-approve pass handles at most one batch per run
func TestAutoApproveBatchBound(t *testing.T) {
	r := newRCRig()
	ids := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		o := r.shipped(t, true)
		ids = append(ids, r.newReturn(t, o.ID, repository.ReturnStatusPending).ID)
	}
	now := pastDeadline()
	pending := func() int {
		n := 0
		for _, id := range ids {
			if r.ret(t, id).Status == repository.ReturnStatusPending {
				n++
			}
		}
		return n
	}
	if n := r.sweepApprove(t, now); n != rcBatch || pending() != 150 {
		t.Fatalf("run 1: approved %d, pending %d; want %d and 150", n, pending(), rcBatch)
	}
	if n := r.sweepApprove(t, now); n != rcBatch || pending() != 50 {
		t.Fatalf("run 2: approved %d, pending %d; want %d and 50", n, pending(), rcBatch)
	}
	if n := r.sweepApprove(t, now); n != 50 || pending() != 0 {
		t.Fatalf("run 3: approved %d, pending %d; want 50 and 0", n, pending())
	}
}

// Scenario: Order receipt: A passed ship_by changes nothing
func TestPassedShipByDoesNotChangeOrder(t *testing.T) {
	r := newRCRig()
	ctx := context.Background()
	o, err := r.orders.CreateOrder(ctx, repository.Order{BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 5000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	paid, err := r.orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, []repository.OrderStatus{repository.OrderStatusPending}, "")
	if err != nil {
		t.Fatal(err)
	}
	if paid.ShipBy == nil {
		t.Fatal("setup: payment must stamp ship_by")
	}
	rows := len(r.outbox.EnqueuedRows())
	r.svc.SweepLifecycle(ctx, paid.ShipBy.Add(24*time.Hour))
	got := r.order(t, o.ID)
	if got.Status != repository.OrderStatusPaid || !got.UpdatedAt.Equal(paid.UpdatedAt) || len(r.outbox.EnqueuedRows()) != rows {
		t.Fatalf("a passed ship_by must change nothing: %+v, outbox %d -> %d", got, rows, len(r.outbox.EnqueuedRows()))
	}
}

// Scenarios: Order receipt: Unconfirmed shipped order completes after the window;
// Unanswered return is approved after the window (both passes run from SweepLifecycle)
func TestSweepLifecycleRunsBothPasses(t *testing.T) {
	r := newRCRig()
	a := r.shipped(t, true)
	b := r.shipped(t, true)
	ret := r.newReturn(t, b.ID, repository.ReturnStatusPending)
	r.svc.SweepLifecycle(context.Background(), pastDeadline())
	if r.order(t, a.ID).Status != repository.OrderStatusCompleted {
		t.Fatal("SweepLifecycle must auto-complete the unblocked shipped order")
	}
	if r.ret(t, ret.ID).Status != repository.ReturnStatusApproved {
		t.Fatal("SweepLifecycle must auto-approve the unanswered return")
	}
}
