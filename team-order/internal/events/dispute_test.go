package events_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	engagementv1 "github.com/buidangphuc/team-order/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

const (
	disputeType = "platform.engagement.v1.DisputeResolved"
	orderTotal  = int64(1000000)
)

type dAudit struct {
	mu     sync.Mutex
	calls  []auditx.Event
	times  []time.Time
	failN  int
	err    error
	during func()
}

func (a *dAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	a.mu.Lock()
	a.calls = append(a.calls, ev)
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

func (a *dAudit) gaps() []time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []time.Duration
	for i := 1; i < len(a.times); i++ {
		out = append(out, a.times[i].Sub(a.times[i-1]))
	}
	return out
}

type dReturns struct {
	*store.PostgresReturnRepository
	mu          sync.Mutex
	transitions [][2]store.ReturnStatus
	reapproves  int
	beforeTrans func(id string, from, to store.ReturnStatus)
}

func (r *dReturns) TransitionReturn(ctx context.Context, id string, from, to store.ReturnStatus) (store.OrderReturn, error) {
	r.mu.Lock()
	r.transitions = append(r.transitions, [2]store.ReturnStatus{from, to})
	hook := r.beforeTrans
	r.mu.Unlock()
	if hook != nil {
		hook(id, from, to)
	}
	return r.PostgresReturnRepository.TransitionReturn(ctx, id, from, to)
}

func (r *dReturns) ReapproveReturn(ctx context.Context, id string, total int64) (store.OrderReturn, error) {
	r.mu.Lock()
	r.reapproves++
	r.mu.Unlock()
	return r.PostgresReturnRepository.ReapproveReturn(ctx, id, total)
}

type dEnv struct {
	t       *testing.T
	rig     *rig
	returns *dReturns
	audit   *dAudit
	h       *events.DisputeHandler
}

func newDEnv(t *testing.T) *dEnv {
	t.Helper()
	r := newRig(t)
	e := &dEnv{t: t, rig: r, returns: &dReturns{PostgresReturnRepository: r.returns}, audit: &dAudit{}}
	h, err := events.NewDisputeHandler(r.orders, e.returns, auditx.NewGate(e.audit, quietLogger()), r.pool, quietLogger())
	if err != nil {
		t.Fatalf("NewDisputeHandler: %v", err)
	}
	e.h = h
	return e
}

func (e *dEnv) order(id string, paidOnline bool) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.rig.orders.CreateOrder(ctx, store.Order{ID: id, BuyerID: "buyer-1", SellerID: "seller-1", TotalAmount: orderTotal}); err != nil {
		e.t.Fatal(err)
	}
	from := store.OrderStatusPending
	if paidOnline {
		e.rig.pay(id)
		from = store.OrderStatusPaid
	}
	if _, err := e.rig.orders.UpdateOrderStatusFrom(ctx, id, store.OrderStatusShipped, []store.OrderStatus{from}, "TRK"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *dEnv) ret(id, orderID string, amount int64, st store.ReturnStatus) {
	e.t.Helper()
	if _, err := e.returns.CreateReturn(context.Background(), store.OrderReturn{
		ID: id, OrderID: orderID, BuyerID: "buyer-1", SellerID: "seller-1", Reason: "broken", RefundAmount: amount, Status: st,
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *dEnv) status(id string) store.ReturnStatus {
	e.t.Helper()
	r, err := e.returns.GetReturn(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.Status
}

func (e *dEnv) outboxTotal() int {
	e.t.Helper()
	return e.rig.count(`SELECT count(*) FROM order_outbox_events WHERE event_type = $1`, events.ReturnRefundedEventType)
}

func (e *dEnv) refundRows(returnID string) []*orderv1.ReturnRefunded {
	e.t.Helper()
	rows, err := e.rig.pool.Query(context.Background(),
		`SELECT payload FROM order_outbox_events WHERE event_type = $1 ORDER BY seq`, events.ReturnRefundedEventType)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []*orderv1.ReturnRefunded
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			e.t.Fatal(err)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(payload, &env); err != nil {
			e.t.Fatal(err)
		}
		var rr orderv1.ReturnRefunded
		if err := proto.Unmarshal(env.GetPayload(), &rr); err != nil {
			e.t.Fatal(err)
		}
		if rr.GetReturnId() == returnID {
			out = append(out, &rr)
		}
	}
	return out
}

func (e *dEnv) processed(eventID string) bool {
	e.t.Helper()
	return e.rig.processedRows(disputeConsumerName, eventID) > 0
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

func (d dEvt) raw(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&engagementv1.DisputeResolved{
		DisputeId: d.disputeID, OrderId: d.orderID, ReturnId: d.returnID,
		Status: d.status, Ruling: d.ruling, ResolverId: d.resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: d.eventID, Type: disputeType, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (d dEvt) record(t *testing.T) consume.Record {
	t.Helper()
	return consume.Record{Topic: "engagement.events", Key: []byte(d.disputeID), Value: d.raw(t)}
}

func (e *dEnv) handle(d dEvt) error {
	return e.h.Handle(context.Background(), d.record(e.t))
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func mustPermanent(t *testing.T, err error) {
	t.Helper()
	if err == nil || !errors.Is(err, events.ErrPermanent) {
		t.Fatalf("expected error wrapping ErrPermanent, got %v", err)
	}
}

func mustTransient(t *testing.T, err error) {
	t.Helper()
	if err == nil || errors.Is(err, events.ErrPermanent) {
		t.Fatalf("expected non-permanent error, got %v", err)
	}
}

func (e *dEnv) wantNothingMoved(returnID string, want store.ReturnStatus) {
	e.t.Helper()
	if got := e.status(returnID); got != want {
		e.t.Fatalf("return status = %d, want %d", got, want)
	}
	if n := e.audit.n(); n != 0 {
		e.t.Fatalf("audit calls = %d, want 0", n)
	}
	if n := e.outboxTotal(); n != 0 {
		e.t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: A nil audit gate is refused at construction
func TestNewDisputeHandlerRefusesNilGate(t *testing.T) {
	r := newRig(t)
	h, err := events.NewDisputeHandler(r.orders, r.returns, nil, r.pool, quietLogger())
	if err == nil {
		t.Fatal("NewDisputeHandler must refuse a nil audit gate")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "gate") {
		t.Fatalf("error %q must name the gate", err)
	}
	if h != nil {
		t.Fatal("no handler may be returned with the error")
	}
}

// Scenario: Dispute refund: Rejected return of an online-paid order is refunded
func TestDisputeBuyerFavourRejectedReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
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

// Scenario: Dispute refund: Pending escalated return of an online-paid order is refunded
func TestDisputeBuyerFavourPendingReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 250000, store.ReturnStatusPending)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
}

// Scenario: Dispute refund: Already approved return is refunded without a second approval
func TestDisputeBuyerFavourApprovedReturnRefunded(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 300000, store.ReturnStatusApproved)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.returns.reapproves != 0 {
		t.Fatalf("ReapproveReturn called %d times for an APPROVED return", e.returns.reapproves)
	}
	for _, tr := range e.returns.transitions {
		if tr[0] == store.ReturnStatusPending || tr[0] == store.ReturnStatusRejected {
			t.Fatalf("unexpected transition from %d", tr[0])
		}
	}
}

// Scenario: Dispute refund: Cash-on-delivery return stops at approved
func TestDisputeBuyerFavourCODStopsAtApproved(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", false)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusApproved {
		t.Fatalf("status = %d, want APPROVED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 0 {
		t.Fatalf("refund rows = %d, want 0 for COD", len(rows))
	}
	if e.audit.n() != 1 {
		t.Fatalf("COD change must still be audited once, got %d", e.audit.n())
	}
}

// Scenario: Dispute refund: Return already refunded is left unchanged
func TestDisputeBuyerFavourAlreadyRefundedNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusApproved)
	if _, err := e.returns.PostgresReturnRepository.TransitionReturn(context.Background(), "r-1", store.ReturnStatusApproved, store.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Scenario: The dispute audit event names the resolving admin
func TestDisputeRefundWritesAuditBeforeChange(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusPending)
	var statusAtAudit store.ReturnStatus
	var rowsAtAudit int
	e.audit.during = func() {
		r, _ := e.returns.GetReturn(context.Background(), "r-1")
		statusAtAudit = r.Status
		rowsAtAudit = e.outboxTotal()
	}
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.resolver = "admin-9"

	mustOK(t, e.handle(d))

	if e.audit.n() != 1 {
		t.Fatalf("audit calls = %d, want exactly 1", e.audit.n())
	}
	if statusAtAudit != store.ReturnStatusPending || rowsAtAudit != 0 {
		t.Fatalf("audit must precede the first change: status=%d outboxRows=%d", statusAtAudit, rowsAtAudit)
	}
	c := e.audit.calls[0]
	if c.ActorID != "admin-9" || c.Action != "return.dispute_refund" || c.TargetType != "return_request" || c.TargetID != "r-1" {
		t.Fatalf("unexpected audit event %+v", c)
	}
	want := map[string]string{
		"order_id":    "o-1",
		"dispute_id":  "d-1",
		"ruling":      "DISPUTE_RULING_BUYER_FAVOUR",
		"from_status": "RETURN_STATUS_PENDING",
	}
	for k, v := range want {
		if c.Metadata[k] != v {
			t.Fatalf("metadata[%s] = %q, want %q (all: %v)", k, c.Metadata[k], v, c.Metadata)
		}
	}
}

// Scenario: Dispute refund: Audit unavailable leaves the return unchanged
func TestDisputeRefundAuditDownNoChangeRetried(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	e.audit.err = errors.New("team-audit unreachable")

	err := e.handle(buyerFavour("evt-1", "r-1", "o-1"))

	mustTransient(t, err)
	if got := e.status("r-1"); got != store.ReturnStatusRejected {
		t.Fatalf("status = %d, want REJECTED", got)
	}
	if n := e.outboxTotal(); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
	if e.processed("evt-1") {
		t.Fatal("event id must not be marked processed")
	}
}

// Scenario: Dispute refund: Event without resolver is a permanent error
func TestDisputeRefundMissingResolverPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.resolver = ""

	mustPermanent(t, e.handle(d))
	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}

// Scenario: A return that no longer fits the order is parked at once
func TestDisputeRefundNoRemainderDeadLettered(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-other", "o-1", 700000, store.ReturnStatusApproved)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustPermanent(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRejected {
		t.Fatalf("status = %d, want REJECTED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 0 {
		t.Fatalf("refund rows = %d, want 0", len(rows))
	}
}

// Scenario: Dispute refund: Seller-favour ruling moves no return and no money
func TestDisputeSellerFavourNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.ruling = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR

	mustOK(t, e.handle(d))
	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}

// Scenario: Dispute refund: Rejected dispute moves no return and no money
func TestDisputeRejectedStatusNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)
	d := buyerFavour("evt-1", "r-1", "o-1")
	d.status = engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED

	mustOK(t, e.handle(d))
	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}

// Scenario: Dispute refund: Dispute without a return is ignored
func TestDisputeNoReturnIdNoop(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustOK(t, e.handle(buyerFavour("evt-1", "", "o-1")))
	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}

// Scenario: Dispute refund: Other event types are ignored
func TestDisputeConsumerIgnoresOtherTypes(t *testing.T) {
	e := newDEnv(t)
	value, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-x", Type: "platform.engagement.v1.ReviewCreated", Payload: []byte{0x0a, 0x01, 0x78}})
	if err != nil {
		t.Fatal(err)
	}

	mustOK(t, e.h.Handle(context.Background(), consume.Record{Topic: "engagement.events", Key: []byte("k"), Value: value}))

	if e.processed("evt-x") {
		t.Fatal("an ignored type must not write a dedupe row")
	}
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Scenario: A redelivered dispute event refunds once
func TestDisputeRedeliveryRefundsOnce(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusApproved)
	d := buyerFavour("evt-1", "r-1", "o-1")

	mustOK(t, e.handle(d))
	mustOK(t, e.handle(d))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("ReturnRefunded outbox rows = %d, want exactly 1", len(rows))
	}
	if n := e.rig.processedRows(disputeConsumerName, "evt-1"); n != 1 {
		t.Fatalf("processed_events rows for %s = %d, want exactly 1", disputeConsumerName, n)
	}
}

// Scenario: Dispute refund: Second ruling event on a refunded return is acknowledged
func TestDisputeSecondEventSameReturnRefundsOnce(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

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

// Scenario: Dispute refund: Auto-approve winning the race does not fail the event
func TestDisputePendingCASLostContinues(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusPending)
	var once sync.Once
	e.returns.beforeTrans = func(id string, from, to store.ReturnStatus) {
		if from == store.ReturnStatusPending && to == store.ReturnStatusApproved {
			once.Do(func() {
				if _, err := e.returns.PostgresReturnRepository.TransitionReturn(context.Background(), id, from, to); err != nil {
					t.Error(err)
				}
			})
		}
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
}

// Scenario: Dispute refund: Seller refund winning the race is acknowledged
func TestDisputeRefundCASLostAcknowledged(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusApproved)
	var once sync.Once
	e.returns.beforeTrans = func(id string, from, to store.ReturnStatus) {
		if from == store.ReturnStatusApproved && to == store.ReturnStatusRefunded {
			once.Do(func() {
				if _, err := e.returns.PostgresReturnRepository.TransitionReturn(context.Background(), id, from, to); err != nil {
					t.Error(err)
				}
			})
		}
	}

	mustOK(t, e.handle(buyerFavour("evt-1", "r-1", "o-1")))

	if got := e.status("r-1"); got != store.ReturnStatusRefunded {
		t.Fatalf("status = %d, want REFUNDED", got)
	}
	if rows := e.refundRows("r-1"); len(rows) != 1 {
		t.Fatalf("refund rows = %d, want 1", len(rows))
	}
	if !e.processed("evt-1") {
		t.Fatal("event must be acknowledged")
	}
}

// Scenario: Dispute refund: Unknown return is a permanent error
func TestDisputeUnknownReturnPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)

	mustPermanent(t, e.handle(buyerFavour("evt-1", "r-missing", "o-1")))
	if e.audit.n() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.n())
	}
}

// Scenario: Dispute refund: Malformed envelope is dead-lettered
func TestDisputeMalformedRecordPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustPermanent(t, e.h.Handle(context.Background(), consume.Record{Topic: "engagement.events", Key: []byte("d-1"), Value: []byte{0xff, 0xff, 0xff}}))

	badPayload, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-bad", Type: disputeType, Payload: []byte{0xff, 0xff}})
	if err != nil {
		t.Fatal(err)
	}
	mustPermanent(t, e.h.Handle(context.Background(), consume.Record{Topic: "engagement.events", Key: []byte("d-1"), Value: badPayload}))

	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}

// Scenario: Dispute refund: Return linked to another order is refused
func TestDisputeOrderMismatchPermanent(t *testing.T) {
	e := newDEnv(t)
	e.order("o-1", true)
	e.order("o-2", true)
	e.ret("r-1", "o-1", 400000, store.ReturnStatusRejected)

	mustPermanent(t, e.handle(buyerFavour("evt-1", "r-1", "o-2")))
	e.wantNothingMoved("r-1", store.ReturnStatusRejected)
}
