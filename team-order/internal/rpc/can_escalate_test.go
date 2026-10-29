package rpc_test

import (
	"testing"
	"time"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

func (e *windowEnv) newReturn(t *testing.T, orderID string) string {
	t.Helper()
	resp, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CreateReturnRequestRequest{OrderId: orderID, Reason: "broken", RefundAmount: 100000})
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetReturnRequest().GetId()
}

func (e *windowEnv) setTo(t *testing.T, id string, sts ...orderv1.ReturnStatus) {
	t.Helper()
	for _, st := range sts {
		if _, err := e.h.UpdateReturnStatus(incomingPrincipalCtx("seller_1", "seller"), &orderv1.UpdateReturnStatusRequest{Id: id, Status: st}); err != nil {
			t.Fatalf("to %v: %v", st, err)
		}
	}
}

func (e *windowEnv) get(t *testing.T, id string) *orderv1.OrderReturn {
	t.Helper()
	resp, err := e.h.GetReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.GetReturnRequestRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetReturnRequest()
}

// Scenario: Order receipt: A rejected return on a shipped order can escalate
func TestCanEscalateRejectedOnShipped(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	id := e.newReturn(t, o.ID)
	e.setTo(t, id, orderv1.ReturnStatus_RETURN_STATUS_REJECTED)
	r := e.get(t, id)
	if r.GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_REJECTED || !r.GetCanEscalate() {
		t.Fatalf("want REJECTED with can_escalate true, got %v %v", r.GetStatus(), r.GetCanEscalate())
	}
}

// Scenario: Order receipt: A rejected return on a shipped order can escalate (UpdateReturnStatus response)
func TestCanEscalateOnUpdateReturnStatusResponse(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	id := e.newReturn(t, o.ID)
	resp, err := e.h.UpdateReturnStatus(incomingPrincipalCtx("seller_1", "seller"),
		&orderv1.UpdateReturnStatusRequest{Id: id, Status: orderv1.ReturnStatus_RETURN_STATUS_REJECTED})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetReturnRequest().GetCanEscalate() {
		t.Fatal("UpdateReturnStatus response must carry can_escalate true")
	}
}

// Scenario: Order receipt: A rejected return inside the window after completion can escalate
func TestCanEscalateRejectedInsideWindow(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusCompleted)
	e.now = o.ReturnWindowEndsAt.Add(-time.Second)
	id := e.newReturn(t, o.ID)
	e.setTo(t, id, orderv1.ReturnStatus_RETURN_STATUS_REJECTED)
	if !e.get(t, id).GetCanEscalate() {
		t.Fatal("want can_escalate true inside the window")
	}
}

// Scenario: Order receipt: A rejected return cannot escalate after the window closes
func TestCanEscalateFalseAfterWindowCloses(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusCompleted)
	e.now = o.ReturnWindowEndsAt.Add(-time.Second)
	id := e.newReturn(t, o.ID)
	e.setTo(t, id, orderv1.ReturnStatus_RETURN_STATUS_REJECTED)
	if !e.get(t, id).GetCanEscalate() {
		t.Fatal("precondition: true inside the window")
	}
	e.now = o.ReturnWindowEndsAt.Add(time.Second)
	r := e.get(t, id)
	if r.GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_REJECTED || r.GetCanEscalate() {
		t.Fatalf("want REJECTED with can_escalate false after window, got %v", r.GetCanEscalate())
	}
}

// Scenario: Order receipt: A return that is not rejected cannot escalate
func TestCanEscalateFalseUnlessRejected(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	pending := e.newReturn(t, o.ID)
	approved := e.newReturn(t, o.ID)
	e.setTo(t, approved, orderv1.ReturnStatus_RETURN_STATUS_APPROVED)
	refunded := e.newReturn(t, o.ID)
	e.setTo(t, refunded, orderv1.ReturnStatus_RETURN_STATUS_APPROVED, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED)
	rejected := e.newReturn(t, o.ID)
	e.setTo(t, rejected, orderv1.ReturnStatus_RETURN_STATUS_REJECTED)
	// control: the same order does escalate a rejected return, so false above is not vacuous
	if !e.get(t, rejected).GetCanEscalate() {
		t.Fatal("control: rejected return on shipped order must escalate")
	}
	for name, id := range map[string]string{"pending": pending, "approved": approved, "refunded": refunded} {
		if e.get(t, id).GetCanEscalate() {
			t.Fatalf("%s return must not escalate", name)
		}
	}
}

// Scenario: Order receipt: ListOrderReturns computes can_escalate per return
func TestListOrderReturnsCanEscalatePerReturn(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	pending := e.newReturn(t, o.ID)
	rejected := e.newReturn(t, o.ID)
	e.setTo(t, rejected, orderv1.ReturnStatus_RETURN_STATUS_REJECTED)
	resp, err := e.h.ListOrderReturns(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.ListOrderReturnsRequest{OrderId: o.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range resp.GetReturns() {
		got[r.GetId()] = r.GetCanEscalate()
	}
	if len(got) != 2 || !got[rejected] || got[pending] {
		t.Fatalf("want rejected=true pending=false, got %v", got)
	}
}

// Scenario: Order receipt: A client-sent can_escalate is ignored
// CreateReturnRequestRequest has no can_escalate field, so the wire cannot carry it;
// the created PENDING return must read false, and a REJECTED one must be server computed.
func TestCreateReturnIgnoresClientCanEscalate(t *testing.T) {
	e := newWindowEnv(t)
	o := e.order(t, store.OrderStatusShipped)
	resp, err := e.h.CreateReturnRequest(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CreateReturnRequestRequest{OrderId: o.ID, Reason: "broken", RefundAmount: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_PENDING || resp.GetReturnRequest().GetCanEscalate() {
		t.Fatalf("want PENDING with can_escalate false, got %v", resp.GetReturnRequest())
	}
}
