package consumer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-order/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
)

const (
	disputeConsumerName = "team-order.dispute"
	disputeType         = "platform.engagement.v1.DisputeResolved"
	orderTotal          = int64(1000000)
)

type dAuditCall struct {
	Actor, Action, TargetType, TargetID string
	Meta                                map[string]string
}

type dAudit struct {
	mu     sync.Mutex
	calls  []dAuditCall
	times  []time.Time
	failN  int
	err    error
	during func()
}

func (a *dAudit) WriteAdminEvent(_ context.Context, actorID, action, targetType, targetID string, meta map[string]string) error {
	a.mu.Lock()
	a.calls = append(a.calls, dAuditCall{actorID, action, targetType, targetID, meta})
	a.times = append(a.times, time.Now())
	n := len(a.calls)
	failN, err, during := a.failN, a.err, a.during
	a.mu.Unlock()
	if during != nil {
		during()
	}
	if failN > 0 {
		if n <= failN {
			return errors.New("team-audit unreachable")
		}
		return nil
	}
	return err
}

func (a *dAudit) n() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

type dReturns struct {
	*repository.InMemoryReturnRepository
	mu           sync.Mutex
	transitions  [][2]repository.ReturnStatus
	reapproves   int
	beforeTrans  func(id string, from, to repository.ReturnStatus)
	beforeReappr func(id string)
}

func (r *dReturns) TransitionReturn(ctx context.Context, id string, from, to repository.ReturnStatus) (repository.OrderReturn, error) {
	r.mu.Lock()
	r.transitions = append(r.transitions, [2]repository.ReturnStatus{from, to})
	hook := r.beforeTrans
	r.mu.Unlock()
	if hook != nil {
		hook(id, from, to)
	}
	return r.InMemoryReturnRepository.TransitionReturn(ctx, id, from, to)
}

func (r *dReturns) ReapproveReturn(ctx context.Context, id string, total int64) (repository.OrderReturn, error) {
	r.mu.Lock()
	r.reapproves++
	hook := r.beforeReappr
	r.mu.Unlock()
	if hook != nil {
		hook(id)
	}
	return r.InMemoryReturnRepository.ReapproveReturn(ctx, id, total)
}

type dEnv struct {
	t       *testing.T
	orders  *repository.InMemoryOrderRepository
	returns *dReturns
	outbox  *repository.InMemoryOutboxRepository
	audit   *dAudit
	dedupe  repository.ProcessedEventRepository
	c       *consumer.DisputeConsumer
}

func newDEnv(t *testing.T) *dEnv {
	t.Helper()
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	inner := repository.NewInMemoryReturnRepository(
		repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	e := &dEnv{
		t: t, orders: orders, outbox: outbox,
		returns: &dReturns{InMemoryReturnRepository: inner},
		audit:   &dAudit{},
		dedupe:  repository.NewInMemoryProcessedEventRepository(),
	}
	e.c = consumer.NewDisputeConsumer(e.orders, e.returns, e.audit, e.dedupe, nil)
	return e
}

func (e *dEnv) order(id string, paidOnline bool) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.orders.CreateOrder(ctx, repository.Order{ID: id, BuyerID: "buyer-1", SellerID: "seller-1", TotalAmount: orderTotal}); err != nil {
		e.t.Fatal(err)
	}
	from := repository.OrderStatusPending
	if paidOnline {
		if _, err := e.orders.UpdateOrderStatusFrom(ctx, id, repository.OrderStatusPaid, []repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
			e.t.Fatal(err)
		}
		from = repository.OrderStatusPaid
	}
	if _, err := e.orders.UpdateOrderStatusFrom(ctx, id, repository.OrderStatusShipped, []repository.OrderStatus{from}, "TRK"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *dEnv) ret(id, orderID string, amount int64, st repository.ReturnStatus) {
	e.t.Helper()
	if _, err := e.returns.CreateReturn(context.Background(), repository.OrderReturn{
		ID: id, OrderID: orderID, BuyerID: "buyer-1", SellerID: "seller-1", Reason: "broken", RefundAmount: amount, Status: st,
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *dEnv) status(id string) repository.ReturnStatus {
	e.t.Helper()
	r, err := e.returns.GetReturn(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.Status
}

func (e *dEnv) refundRows(returnID string) []orderv1.ReturnRefunded {
	e.t.Helper()
	var out []orderv1.ReturnRefunded
	for _, row := range e.outbox.EnqueuedRows() {
		if row.EventType != events.ReturnRefundedEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			e.t.Fatal(err)
		}
		var rr orderv1.ReturnRefunded
		if err := proto.Unmarshal(env.GetPayload(), &rr); err != nil {
			e.t.Fatal(err)
		}
		if rr.GetReturnId() == returnID {
			out = append(out, orderv1.ReturnRefunded{ReturnId: rr.GetReturnId(), RefundAmount: rr.GetRefundAmount()})
		}
	}
	return out
}

func (e *dEnv) processed(eventID string) bool {
	e.t.Helper()
	ok, err := e.dedupe.IsProcessed(context.Background(), disputeConsumerName, eventID)
	if err != nil {
		e.t.Fatal(err)
	}
	return ok
}

type dEvt struct {
	eventID, disputeID, orderID, returnID, resolver string
	status                                          engagementv1.DisputeStatus
	ruling                                          engagementv1.DisputeRuling
}

func buyerFavour(eventID, returnID, orderID string) dEvt {
	return dEvt{
		eventID: eventID, disputeID: "d-1", orderID: orderID, returnID: returnID, resolver: "admin-1",
		status: engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		ruling: engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR,
	}
}

func (d dEvt) envelope(t *testing.T) *eventsv1.EventEnvelope {
	t.Helper()
	payload, err := proto.Marshal(&engagementv1.DisputeResolved{
		DisputeId: d.disputeID, OrderId: d.orderID, ReturnId: d.returnID,
		Status: d.status, Ruling: d.ruling, ResolverId: d.resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &eventsv1.EventEnvelope{EventId: d.eventID, Type: disputeType, Payload: payload}
}

func (d dEvt) raw(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(d.envelope(t))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *dEnv) handle(d dEvt) error {
	return e.c.HandleEnvelope(context.Background(), d.envelope(e.t))
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func mustPermanent(t *testing.T, err error) {
	t.Helper()
	if err == nil || !errors.Is(err, consumer.ErrPermanent) {
		t.Fatalf("expected error wrapping ErrPermanent, got %v", err)
	}
}

func mustTransient(t *testing.T, err error) {
	t.Helper()
	if err == nil || errors.Is(err, consumer.ErrPermanent) {
		t.Fatalf("expected non-permanent error, got %v", err)
	}
}

func (e *dEnv) wantNothingMoved(returnID string, want repository.ReturnStatus) {
	e.t.Helper()
	if got := e.status(returnID); got != want {
		e.t.Fatalf("return status = %d, want %d", got, want)
	}
	if n := e.audit.n(); n != 0 {
		e.t.Fatalf("audit calls = %d, want 0", n)
	}
	if rows := e.outbox.EnqueuedRows(); len(rows) != 0 {
		e.t.Fatalf("outbox rows = %d, want 0", len(rows))
	}
}

// Dispute refund: Rejected return of an online-paid order is refunded
func TestDisputeBuyerFavourRejectedReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	rows := e.refundRows("r-1")
	if len(rows) != 1 || rows[0].GetRefundAmount() != 400000 {
		t.Fatalf("refund rows = %+v, want exactly one of 400000", rows)
	}
	if !e.processed("evt-1") {
		t.Fatal("event id not recorded as processed after the effect")
	}
}

// Dispute refund: Pending escalated return of an online-paid order is refunded
func TestDisputeBuyerFavourPendingReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 250000, repository.ReturnStatusPending)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
}

// Dispute refund: Already approved return is refunded without a second approval
func TestDisputeBuyerFavourApprovedReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 300000, repository.ReturnStatusApproved)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.returns.reapproves != 0 {
		t.Fatalf("ReapproveReturn called %d times for an APPROVED return", e.returns.reapproves)
	}
	for _, tr := range e.returns.transitions {
		if tr[0] == repository.ReturnStatusPending || tr[0] == repository.ReturnStatusRejected {
			t.Fatalf("unexpected transition from %d", tr[0])
		}
	}
}

// Dispute refund: Cash-on-delivery return stops at approved
func TestDisputeBuyerFavourCODStopsAtApproved(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", false)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusApproved {
		t.Fatalf("status = %d, want APPROVED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 0 {
		t.Fatalf("refund rows = %d, want 0 for COD", len(rows))
	}
	if e.audit.n() != 1 {
		t.Fatalf("COD change must still be audited once, got %d", e.audit.n())
	}
}

// Dispute refund: Return already refunded is left unchanged
func TestDisputeBuyerFavourAlreadyRefundedNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusApproved)
	if _, err := e.returns.InMemoryReturnRepository.TransitionReturn(context.Background(), "r-1", repository.ReturnStatusApproved, repository.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Dispute refund: Audit event names the resolving admin
func TestDisputeRefundWritesAuditBeforeChange(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	var statusAtAudit repository.ReturnStatus
	var rowsAtAudit int
	e.audit.during = func() {
		r, _ := e.returns.InMemoryReturnRepository.GetReturn(context.Background(), "r-1")
		statusAtAudit = r.Status
		rowsAtAudit = len(e.outbox.EnqueuedRows())
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if e.audit.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", e.audit.n())
	}
	if statusAtAudit != repository.ReturnStatusRejected || rowsAtAudit != 0 {
		t.Fatalf("audit must precede the first change: status=%d outboxRows=%d", statusAtAudit, rowsAtAudit)
	}
	c := e.audit.calls[0]
	if c.Actor != "admin-1" || c.Action != "return.dispute_refund" || c.TargetType != "return_request" || c.TargetID != "r-1" {
		t.Fatalf("unexpected audit call %+v", c)
	}
	want := map[string]string{
		"order_id":    "o-1",
		"dispute_id":  "d-1",
		"ruling":      "DISPUTE_RULING_BUYER_FAVOUR",
		"from_status": "RETURN_STATUS_REJECTED",
	}
	for k, v := range want {
		if c.Meta[k] != v {
			t.Fatalf("metadata[%s] = %q, want %q (all: %v)", k, c.Meta[k], v, c.Meta)
		}
	}
}

// Dispute refund: Audit unavailable leaves the return unchanged
func TestDisputeRefundAuditDownNoChangeRetried(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	e.audit.err = errors.New("team-audit unreachable")

	err := e.handle(buyerFavour("evt-1", "r-1", "o-1"))

	mustTransient(t, err)
	if got := e.status("r-1"); got != repository.ReturnStatusRejected {
		t.Fatalf("status = %d, want REJECTED", got)
	}
	if rows := e.outbox.EnqueuedRows(); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(rows))
	}
	if e.processed("evt-1") {
		t.Fatal("event id must not be marked processed")
	}
}

// Dispute refund: Missing audit client refuses the change
func TestDisputeRefundNilAuditWriterNoChange(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	c := consumer.NewDisputeConsumer(e.orders, e.returns, nil, e.dedupe, nil)

	err := c.HandleEnvelope(context.Background(), buyerFavour("evt-1", "r-1", "o-1").envelope(t))

	mustTransient(t, err)
	if got := e.status("r-1"); got != repository.ReturnStatusRejected {
		t.Fatalf("status = %d, want REJECTED", got)
	}
	if rows := e.outbox.EnqueuedRows(); len(rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(rows))
	}
}

// Dispute refund: Event without resolver is a permanent error
func TestDisputeRefundMissingResolverPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.resolver = ""

	mustPermanent(t, e.handle(d))
	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

// Dispute refund: Non-fitting return is dead-lettered for CS
func TestDisputeRefundNoRemainderDeadLettered(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-other", "o-1", 700000, repository.ReturnStatusApproved)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")

	mustPermanent(t, e.handle(d))
	if got := e.status("r-1"); got != repository.ReturnStatusRejected {
		t.Fatalf("status = %d, want REJECTED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 0 {
		t.Fatalf("refund rows = %d, want 0", len(rows))
	}

	reader := newDReader(d.raw(t))
	dlq := &dDLQ{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader.onCommit = cancel
	_ = e.c.Run(ctx, reader, dlq, consumer.RunConfig{DLQTopic: "engagement.events.order.dlq", BaseBackoff: time.Millisecond})

	if len(dlq.produced) != 1 || dlq.topics[0] != "engagement.events.order.dlq" {
		t.Fatalf("dlq produced %d records to %v, want 1 to engagement.events.order.dlq", len(dlq.produced), dlq.topics)
	}
	if reader.committedCount() != 1 {
		t.Fatalf("commits = %d, want 1", reader.committedCount())
	}
}

// Dispute refund: Seller-favour ruling moves no return and no money
func TestDisputeSellerFavourNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.ruling = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR

	mustOK(t, e.handle(d))
	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

// Dispute refund: Rejected dispute moves no return and no money
func TestDisputeRejectedStatusNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.status = engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED

	mustOK(t, e.handle(d))
	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

// Dispute refund: Dispute without a return is ignored
func TestDisputeNoReturnIdNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "", "o-1")

	mustOK(t, e.handle(d))
	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

// Dispute refund: Other event types are ignored
func TestDisputeConsumerIgnoresOtherTypes(t *testing.T) {
	e := newDEnv(t)
	env := &eventsv1.EventEnvelope{EventId: "evt-x", Type: "platform.engagement.v1.ReviewCreated", Payload: []byte{0x0a, 0x01, 0x78}}

	mustOK(t, e.c.HandleEnvelope(context.Background(), env))

	if e.processed("evt-x") {
		t.Fatal("an ignored type must not write a dedupe row")
	}
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Dispute refund: Redelivered event refunds once
func TestDisputeRedeliveryRefundsOnce(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")

	mustOK(t, e.handle(d))
	mustOK(t, e.handle(d))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.audit.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", e.audit.n())
	}
}

// Dispute refund: Second ruling event on a refunded return is acknowledged
func TestDisputeSecondEventSameReturnRefundsOnce(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))
	mustOK(t, e.handle(buyerFavour("evt-2", "r-1", "o-1")))

	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.audit.n() != 1 {
		t.Fatalf("audit calls = %d, want 1 (none for the second event)", e.audit.n())
	}
	if !e.processed("evt-2") {
		t.Fatal("second event must be acknowledged and recorded")
	}
}

// Dispute refund: Auto-approve winning the race does not fail the event
func TestDisputePendingCASLostContinues(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusPending)
	var once sync.Once
	e.returns.beforeTrans = func(id string, from, to repository.ReturnStatus) {
		if from == repository.ReturnStatusPending && to == repository.ReturnStatusApproved {
			once.Do(func() {
				if _, err := e.returns.InMemoryReturnRepository.TransitionReturn(context.Background(), id, from, to); err != nil {
					t.Error(err)
				}
			})
		}
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
}

// Dispute refund: Seller refund winning the race is acknowledged
func TestDisputeRefundCASLostAcknowledged(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusApproved)
	var once sync.Once
	e.returns.beforeTrans = func(id string, from, to repository.ReturnStatus) {
		if from == repository.ReturnStatusApproved && to == repository.ReturnStatusRefunded {
			once.Do(func() {
				if _, err := e.returns.InMemoryReturnRepository.TransitionReturn(context.Background(), id, from, to); err != nil {
					t.Error(err)
				}
			})
		}
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if !e.processed("evt-1") {
		t.Fatal("event must be acknowledged")
	}
}

// Dispute refund: Unknown return is a permanent error
func TestDisputeUnknownReturnPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)

	mustPermanent(t, e.handle(buyerFavour("evt-1", "r-missing", "o-1")))
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Dispute refund: Malformed envelope is dead-lettered
func TestDisputeMalformedRecordPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)

	mustPermanent(t, e.c.HandleRaw(context.Background(), []byte{0xff, 0xff, 0xff}))

	badPayload, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-bad", Type: disputeType, Payload: []byte{0xff, 0xff}})
	if err != nil {
		t.Fatal(err)
	}
	mustPermanent(t, e.c.HandleRaw(context.Background(), badPayload))

	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

// Dispute refund: Return linked to another order is refused
func TestDisputeOrderMismatchPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.order("o-2", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)

	mustPermanent(t, e.handle(buyerFavour("evt-1", "r-1", "o-2")))
	e.wantNothingMoved("r-1", repository.ReturnStatusRejected)
}

type dReader struct {
	mu        sync.Mutex
	records   []consumer.Record
	idx       int
	committed []consumer.Record
	onCommit  func()
}

func newDReader(values ...[]byte) *dReader {
	r := &dReader{}
	for _, v := range values {
		r.records = append(r.records, consumer.Record{Key: "d-1", Value: v})
	}
	return r
}

func (r *dReader) Fetch(ctx context.Context) (consumer.Record, error) {
	r.mu.Lock()
	if r.idx >= len(r.records) {
		r.mu.Unlock()
		<-ctx.Done()
		return consumer.Record{}, ctx.Err()
	}
	rec := r.records[r.idx]
	r.idx++
	r.mu.Unlock()
	return rec, nil
}

func (r *dReader) Commit(_ context.Context, rec consumer.Record) error {
	r.mu.Lock()
	r.committed = append(r.committed, rec)
	hook := r.onCommit
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (r *dReader) committedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.committed)
}

type dDLQ struct {
	mu       sync.Mutex
	produced []consumer.Record
	topics   []string
	err      error
}

func (d *dDLQ) Produce(_ context.Context, topic string, rec consumer.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.produced = append(d.produced, rec)
	d.topics = append(d.topics, topic)
	return nil
}

// Dispute refund: Audit outage longer than the retry window is not dead-lettered
func TestDisputeRunRetriesTransientWithoutDLQ(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, repository.ReturnStatusRejected)
	e.audit.failN = 20
	reader := newDReader(buyerFavour("evt-1", "r-1", "o-1").raw(t))
	dlq := &dDLQ{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader.onCommit = cancel

	var commitsBeforeSuccess int
	e.audit.during = func() {
		if e.audit.n() <= 20 {
			commitsBeforeSuccess += reader.committedCount()
		}
	}

	maxBackoff := 50 * time.Millisecond
	_ = e.c.Run(ctx, reader, dlq, consumer.RunConfig{
		DLQTopic:                   "engagement.events.order.dlq",
		MaxAttempts:                3,
		BaseBackoff:                10 * time.Millisecond,
		MaxBackoff:                 maxBackoff,
		RetryTransientUntilApplied: true,
	})

	if len(dlq.produced) != 0 {
		t.Fatalf("dlq produced %d records, want 0", len(dlq.produced))
	}
	if e.audit.n() != 21 {
		t.Fatalf("audit attempts = %d, want 21", e.audit.n())
	}
	if commitsBeforeSuccess != 0 {
		t.Fatal("offset committed before the successful attempt")
	}
	if reader.committedCount() != 1 {
		t.Fatalf("commits = %d, want exactly 1", reader.committedCount())
	}
	if got := e.status("r-1"); got != repository.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	slack := 100 * time.Millisecond
	for i := 1; i < len(e.audit.times); i++ {
		if gap := e.audit.times[i].Sub(e.audit.times[i-1]); gap > maxBackoff+slack {
			t.Fatalf("retry delay %d = %v exceeds MaxBackoff %v", i, gap, maxBackoff)
		}
	}
}

// Dispute refund: Offset is not committed when the DLQ write fails
func TestDisputeRunNoCommitWhenDLQFails(t *testing.T) {
	e := newDEnv(t)
	reader := newDReader([]byte{0xff, 0xff, 0xff})
	dlq := &dDLQ{err: errors.New("dlq broker down")}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = e.c.Run(ctx, reader, dlq, consumer.RunConfig{
		DLQTopic:                   "engagement.events.order.dlq",
		BaseBackoff:                time.Millisecond,
		MaxBackoff:                 time.Millisecond,
		RetryTransientUntilApplied: true,
	})

	if reader.committedCount() != 0 {
		t.Fatalf("commits = %d, want 0 when the DLQ write fails", reader.committedCount())
	}
}
