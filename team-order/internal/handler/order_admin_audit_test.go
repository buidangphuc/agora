package handler_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// order-admin-audit: every admin-path mutation writes one audit event before the
// change; failure or a missing client refuses the call with Unavailable.

type auditCall struct {
	Actor, Action, TargetType, TargetID string
	Meta                                map[string]string
}

// fakeAudit is a recording AuditWriter. A zero value is permissive (always succeeds).
// during runs inside the call, i.e. at the moment the change has NOT happened yet.
type fakeAudit struct {
	mu     sync.Mutex
	calls  []auditCall
	err    error
	during func()
}

func (f *fakeAudit) WriteAdminEvent(_ context.Context, actorID, action, targetType, targetID string, metadata map[string]string) error {
	f.mu.Lock()
	f.calls = append(f.calls, auditCall{actorID, action, targetType, targetID, metadata})
	f.mu.Unlock()
	if f.during != nil {
		f.during()
	}
	return f.err
}

func (f *fakeAudit) n() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

var errAuditDown = errors.New("team-audit unreachable")

func opts(w *fakeAudit) []handler.Option {
	if w == nil {
		return nil // no audit client configured
	}
	return []handler.Option{handler.WithAuditWriter(w)}
}

func adminUser() context.Context  { return scopedUser("admin_1", "admin", "order.admin") }
func sellerUser() context.Context { return scopedUser("sa", "order.write") }
func buyerUser() context.Context  { return scopedUser("buyer_1") }

func wantAudit(t *testing.T, w *fakeAudit, want auditCall) {
	t.Helper()
	if w.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", w.n())
	}
	if got := w.calls[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("audit call = %+v, want %+v", got, want)
	}
}

// ---------- ForceFailSaga ----------

func forceFailRig(w *fakeAudit, st repository.OrderStatus) (*handler.OrderHandler, *mockOrderServiceRepo) {
	repo := &mockOrderServiceRepo{orders: map[string]repository.Order{
		"ord_1": {ID: "ord_1", BuyerID: "buyer_1", SellerID: "sa", Status: st},
	}}
	return handler.NewOrderHandler(service.NewOrderService(repo, nil, nil, nil, nil, nil, nil), nil, nil, opts(w)...), repo
}

// Scenario: Order audit: Admin force-fails an order
func TestForceFailSagaWritesAuditEvent(t *testing.T) {
	w := &fakeAudit{}
	h, repo := forceFailRig(w, repository.OrderStatusPending)
	var seen repository.OrderStatus
	w.during = func() { seen = repo.orders["ord_1"].Status }
	if _, err := h.ForceFailSaga(adminUser(), &orderv1.ForceFailSagaRequest{OrderId: "ord_1", FailStep: "payment"}); err != nil {
		t.Fatal(err)
	}
	if repo.orders["ord_1"].Status != repository.OrderStatusCancelled {
		t.Fatalf("order = %v, want Cancelled", repo.orders["ord_1"].Status)
	}
	if seen != repository.OrderStatusPending {
		t.Fatalf("order was %v when the audit was written, want Pending (audit before change)", seen)
	}
	wantAudit(t, w, auditCall{"admin_1", "order.force_fail", "order", "ord_1", map[string]string{"fail_step": "payment"}})
}

// Scenario: Order audit: Empty fail step is recorded as unspecified
func TestForceFailSagaAuditEmptyStepUnspecified(t *testing.T) {
	w := &fakeAudit{}
	h, _ := forceFailRig(w, repository.OrderStatusPending)
	if _, err := h.ForceFailSaga(adminUser(), &orderv1.ForceFailSagaRequest{OrderId: "ord_1"}); err != nil {
		t.Fatal(err)
	}
	wantAudit(t, w, auditCall{"admin_1", "order.force_fail", "order", "ord_1", map[string]string{"fail_step": "unspecified"}})
}

// Scenario: Order audit: Force-fail refused while audit is down
func TestForceFailSagaAuditDownUnavailableNoChange(t *testing.T) {
	w := &fakeAudit{err: errAuditDown}
	h, repo := forceFailRig(w, repository.OrderStatusPending)
	_, err := h.ForceFailSaga(adminUser(), &orderv1.ForceFailSagaRequest{OrderId: "ord_1", FailStep: "payment"})
	wantCode(t, err, codes.Unavailable)
	if strings.Contains(err.Error(), errAuditDown.Error()) {
		t.Fatalf("cause must not leak to the caller: %v", err)
	}
	if repo.orders["ord_1"].Status != repository.OrderStatusPending {
		t.Fatalf("order = %v, want Pending", repo.orders["ord_1"].Status)
	}
}

// Scenario: Order audit: Force-fail refused without an audit client
func TestForceFailSagaNilAuditWriterUnavailable(t *testing.T) {
	h, repo := forceFailRig(nil, repository.OrderStatusPending)
	_, err := h.ForceFailSaga(adminUser(), &orderv1.ForceFailSagaRequest{OrderId: "ord_1"})
	wantCode(t, err, codes.Unavailable)
	if repo.orders["ord_1"].Status != repository.OrderStatusPending {
		t.Fatalf("order = %v, want Pending", repo.orders["ord_1"].Status)
	}
}

// Scenario: Order audit: Unauthorized force-fail writes no event
func TestForceFailSagaDeniedWritesNoAudit(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"admin without order.admin": {scopedUser("admin_1", "admin"), codes.PermissionDenied},
		"seller":                    {sellerUser(), codes.PermissionDenied},
		"unauthenticated":           {context.Background(), codes.Unauthenticated},
	} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			h, repo := forceFailRig(w, repository.OrderStatusPending)
			_, err := h.ForceFailSaga(tc.ctx, &orderv1.ForceFailSagaRequest{OrderId: "ord_1"})
			wantCode(t, err, tc.want)
			if w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
			if repo.orders["ord_1"].Status != repository.OrderStatusPending {
				t.Fatal("order must be unchanged")
			}
		})
	}
}

// Scenario: Order audit: Invalid or unknown input writes no event
func TestForceFailSagaInvalidInputWritesNoAudit(t *testing.T) {
	for name, tc := range map[string]struct {
		req  *orderv1.ForceFailSagaRequest
		want codes.Code
	}{
		"bad step":      {&orderv1.ForceFailSagaRequest{OrderId: "ord_1", FailStep: "banana"}, codes.InvalidArgument},
		"empty id":      {&orderv1.ForceFailSagaRequest{}, codes.InvalidArgument},
		"unknown order": {&orderv1.ForceFailSagaRequest{OrderId: "nope"}, codes.NotFound},
	} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			h, _ := forceFailRig(w, repository.OrderStatusPending)
			_, err := h.ForceFailSaga(adminUser(), tc.req)
			wantCode(t, err, tc.want)
			if w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Invalid transition keeps the audit row
func TestForceFailSagaInvalidStatusAfterAudit(t *testing.T) {
	w := &fakeAudit{}
	h, repo := forceFailRig(w, repository.OrderStatusCompleted)
	_, err := h.ForceFailSaga(adminUser(), &orderv1.ForceFailSagaRequest{OrderId: "ord_1"})
	wantCode(t, err, codes.FailedPrecondition)
	if w.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", w.n())
	}
	if repo.orders["ord_1"].Status != repository.OrderStatusCompleted {
		t.Fatalf("order = %v, want Completed", repo.orders["ord_1"].Status)
	}
}

// ---------- UpdateOrderStatus ----------

type orderRig struct {
	h      *handler.OrderHandler
	orders *repository.InMemoryOrderRepository
	ships  *repository.InMemoryShipmentRepository
}

func newOrderRig(t *testing.T, w *fakeAudit) orderRig {
	t.Helper()
	orders := repository.NewInMemoryOrderRepository()
	ships := repository.NewInMemoryShipmentRepository()
	for _, o := range []repository.Order{
		{ID: "ord_p", BuyerID: "buyer_1", SellerID: "sa", Status: repository.OrderStatusPaid},
		{ID: "ord_pending", BuyerID: "buyer_1", SellerID: "sa", Status: repository.OrderStatusPending},
	} {
		if _, err := orders.CreateOrder(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}
	svc := service.NewOrderService(orders, nil, nil, ships, nil, nil, nil)
	return orderRig{h: handler.NewOrderHandler(svc, nil, nil, opts(w)...), orders: orders, ships: ships}
}

func (r orderRig) status(t *testing.T, id string) repository.OrderStatus {
	t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o.Status
}

func ship(id string) *orderv1.UpdateOrderStatusRequest {
	return &orderv1.UpdateOrderStatusRequest{Id: id, Status: orderv1.OrderStatus_ORDER_STATUS_SHIPPED}
}

// Scenario: Order audit: Admin ships a seller's order
func TestUpdateOrderStatusAdminWritesAuditEvent(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	var seen repository.OrderStatus
	w.during = func() { seen = r.status(t, "ord_p") }
	if _, err := r.h.UpdateOrderStatus(adminUser(), ship("ord_p")); err != nil {
		t.Fatal(err)
	}
	if got := r.status(t, "ord_p"); got != repository.OrderStatusShipped {
		t.Fatalf("order = %v, want Shipped", got)
	}
	if seen != repository.OrderStatusPaid {
		t.Fatalf("order was %v at audit time, want Paid (audit before change)", seen)
	}
	wantAudit(t, w, auditCall{"admin_1", "order.admin_status_update", "order", "ord_p",
		map[string]string{"seller_id": "sa", "to_status": "ORDER_STATUS_SHIPPED"}})
}

// Scenario: Order audit: Seller updates their own order unaudited
func TestUpdateOrderStatusSellerNotAudited(t *testing.T) {
	for name, w := range map[string]*fakeAudit{"audit down": {err: errAuditDown}, "no audit client": nil} {
		t.Run(name, func(t *testing.T) {
			r := newOrderRig(t, w)
			if _, err := r.h.UpdateOrderStatus(sellerUser(), ship("ord_p")); err != nil {
				t.Fatal(err)
			}
			if got := r.status(t, "ord_p"); got != repository.OrderStatusShipped {
				t.Fatalf("order = %v, want Shipped", got)
			}
			if w != nil && w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Order admin who is also the seller is audited
func TestUpdateOrderStatusOrderAdminSellerIsAudited(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	if _, err := r.h.UpdateOrderStatus(scopedUser("sa", "order.admin"), ship("ord_p")); err != nil {
		t.Fatal(err)
	}
	if w.n() != 1 || w.calls[0].Action != "order.admin_status_update" || w.calls[0].Actor != "sa" {
		t.Fatalf("audit calls = %+v, want one order.admin_status_update by sa", w.calls)
	}
}

// Scenario: Order audit: Status update refused while audit is down
func TestUpdateOrderStatusAuditDownUnavailableNoChange(t *testing.T) {
	w := &fakeAudit{err: errAuditDown}
	r := newOrderRig(t, w)
	_, err := r.h.UpdateOrderStatus(adminUser(), ship("ord_p"))
	wantCode(t, err, codes.Unavailable)
	if got := r.status(t, "ord_p"); got != repository.OrderStatusPaid {
		t.Fatalf("order = %v, want Paid", got)
	}
}

// Scenario: Order audit: Status update refused without an audit client
func TestUpdateOrderStatusNilAuditWriterUnavailable(t *testing.T) {
	r := newOrderRig(t, nil)
	_, err := r.h.UpdateOrderStatus(adminUser(), ship("ord_p"))
	wantCode(t, err, codes.Unavailable)
	if got := r.status(t, "ord_p"); got != repository.OrderStatusPaid {
		t.Fatalf("order = %v, want Paid", got)
	}
}

// Scenario: Order audit: Refused status calls write no event
func TestUpdateOrderStatusRefusedWritesNoAudit(t *testing.T) {
	shipped := orderv1.OrderStatus_ORDER_STATUS_SHIPPED
	for name, tc := range map[string]struct {
		ctx  context.Context
		req  *orderv1.UpdateOrderStatusRequest
		want codes.Code
	}{
		"buyer":              {buyerUser(), ship("ord_p"), codes.PermissionDenied},
		"unauthenticated":    {context.Background(), ship("ord_p"), codes.Unauthenticated},
		"empty id":           {adminUser(), &orderv1.UpdateOrderStatusRequest{Status: shipped}, codes.InvalidArgument},
		"unspecified status": {adminUser(), &orderv1.UpdateOrderStatusRequest{Id: "ord_p"}, codes.InvalidArgument},
		"unknown id":         {adminUser(), ship("nope"), codes.NotFound},
	} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			r := newOrderRig(t, w)
			_, err := r.h.UpdateOrderStatus(tc.ctx, tc.req)
			wantCode(t, err, tc.want)
			if w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Invalid transition by an admin keeps the audit row
func TestUpdateOrderStatusAdminInvalidTransitionAfterAudit(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	_, err := r.h.UpdateOrderStatus(adminUser(), &orderv1.UpdateOrderStatusRequest{Id: "ord_p", Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED})
	wantCode(t, err, codes.FailedPrecondition)
	if w.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", w.n())
	}
	if got := r.status(t, "ord_p"); got != repository.OrderStatusPaid {
		t.Fatalf("order = %v, want Paid", got)
	}
}

// ---------- UpdateReturnStatus ----------

type returnRig struct {
	rmaEnv
	h *handler.OrderHandler
}

func newReturnRig(w *fakeAudit) returnRig {
	outbox := repository.NewInMemoryOutboxRepository()
	orders := repository.NewInMemoryOrderRepository()
	returns := repository.NewInMemoryReturnRepository(repository.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		repository.WithInMemoryReturnOutbox(outbox, orders))
	svc := service.NewOrderService(orders, nil, returns, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil, opts(w)...)
	return returnRig{rmaEnv: rmaEnv{h: h, orders: orders, outbox: outbox}, h: h}
}

func (r returnRig) retStatus(t *testing.T, id string) orderv1.ReturnStatus {
	t.Helper()
	resp, err := r.h.GetReturnRequest(buyerCtxForRMA(), &orderv1.GetReturnRequestRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetReturnRequest().GetStatus()
}

func buyerCtxForRMA() context.Context { return incomingPrincipalCtx("buyer_1", "buyer") }

func rmaSeller() context.Context { return incomingPrincipalCtx("seller_1", "seller") }

// pendingReturn builds a paid+shipped order and a Pending return on it.
func (r returnRig) pendingReturn(t *testing.T) (orderID, retID string) {
	t.Helper()
	o := r.order(t, true)
	return o.ID, r.createReturn(t, o.ID, 200000)
}

// Scenario: Order audit: Admin approves a return
func TestUpdateReturnStatusAdminWritesAuditEvent(t *testing.T) {
	w := &fakeAudit{}
	r := newReturnRig(w)
	orderID, id := r.pendingReturn(t)
	var seen orderv1.ReturnStatus
	w.during = func() { seen = r.retStatus(t, id) }
	if _, err := r.h.UpdateReturnStatus(adminCtx(), &orderv1.UpdateReturnStatusRequest{Id: id, Status: orderv1.ReturnStatus_RETURN_STATUS_APPROVED}); err != nil {
		t.Fatal(err)
	}
	if got := r.retStatus(t, id); got != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
		t.Fatalf("return = %v, want Approved", got)
	}
	if seen != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Fatalf("return was %v at audit time, want Pending (audit before change)", seen)
	}
	wantAudit(t, w, auditCall{"ops", "return.admin_status_update", "return_request", id,
		map[string]string{"order_id": orderID, "to_status": "RETURN_STATUS_APPROVED"}})
}

// Scenario: Order audit: Seller updates their own return unaudited
func TestUpdateReturnStatusSellerNotAudited(t *testing.T) {
	for name, w := range map[string]*fakeAudit{"audit down": {err: errAuditDown}, "no audit client": nil} {
		t.Run(name, func(t *testing.T) {
			r := newReturnRig(w)
			_, id := r.pendingReturn(t)
			if err := r.setStatus(rmaSeller(), id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED); err != nil {
				t.Fatal(err)
			}
			if w != nil && w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Return update refused while audit is down
func TestUpdateReturnStatusAuditDownUnavailableNoChange(t *testing.T) {
	w := &fakeAudit{err: errAuditDown}
	r := newReturnRig(w)
	_, id := r.pendingReturn(t)
	if err := r.setStatus(rmaSeller(), id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED); err != nil {
		t.Fatal(err)
	}
	wantCode(t, r.setStatus(adminCtx(), id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED), codes.Unavailable)
	if got := r.retStatus(t, id); got != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
		t.Fatalf("return = %v, want Approved", got)
	}
	if r.refundRows() != 0 {
		t.Fatalf("ReturnRefunded rows = %d, want 0", r.refundRows())
	}
}

// Scenario: Order audit: Return update refused without an audit client
func TestUpdateReturnStatusNilAuditWriterUnavailable(t *testing.T) {
	r := newReturnRig(nil)
	_, id := r.pendingReturn(t)
	wantCode(t, r.setStatus(adminCtx(), id, orderv1.ReturnStatus_RETURN_STATUS_APPROVED), codes.Unavailable)
	if got := r.retStatus(t, id); got != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Fatalf("return = %v, want Pending", got)
	}
}

// Scenario: Order audit: Refused return calls write no event
func TestUpdateReturnStatusRefusedWritesNoAudit(t *testing.T) {
	approved := orderv1.ReturnStatus_RETURN_STATUS_APPROVED
	for name, tc := range map[string]struct {
		ctx  func() context.Context
		id   func(string) string
		st   orderv1.ReturnStatus
		want codes.Code
	}{
		"buyer":              {buyerCtxForRMA, func(id string) string { return id }, approved, codes.PermissionDenied},
		"unauthenticated":    {context.Background, func(id string) string { return id }, approved, codes.Unauthenticated},
		"empty id":           {adminCtx, func(string) string { return "" }, approved, codes.InvalidArgument},
		"unspecified status": {adminCtx, func(id string) string { return id }, orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, codes.InvalidArgument},
		"unknown id":         {adminCtx, func(string) string { return "nope" }, approved, codes.NotFound},
	} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			r := newReturnRig(w)
			_, id := r.pendingReturn(t)
			wantCode(t, r.setStatus(tc.ctx(), tc.id(id), tc.st), tc.want)
			if w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Invalid return transition by an admin keeps the audit row
func TestUpdateReturnStatusAdminInvalidTransitionAfterAudit(t *testing.T) {
	w := &fakeAudit{}
	r := newReturnRig(w)
	_, id := r.pendingReturn(t)
	wantCode(t, r.setStatus(adminCtx(), id, orderv1.ReturnStatus_RETURN_STATUS_REFUNDED), codes.FailedPrecondition)
	if w.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", w.n())
	}
	if got := r.retStatus(t, id); got != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Fatalf("return = %v, want Pending", got)
	}
}

// ---------- CreateShipment ----------

func shipReq(orderID, carrier string) *orderv1.CreateShipmentRequest {
	return &orderv1.CreateShipmentRequest{OrderId: orderID, Carrier: carrier, TrackingCode: "TRK-SECRET-1"}
}

func (r orderRig) noShipment(t *testing.T, orderID string) {
	t.Helper()
	if _, err := r.ships.GetShipmentByOrderID(context.Background(), orderID); err == nil {
		t.Fatalf("a shipment exists for %s, want none", orderID)
	}
}

// Scenario: Order audit: Admin creates a shipment
func TestCreateShipmentAdminWritesAuditEvent(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	w.during = func() { r.noShipment(t, "ord_p") }
	if _, err := r.h.CreateShipment(adminUser(), shipReq("ord_p", "SPX")); err != nil {
		t.Fatal(err)
	}
	wantAudit(t, w, auditCall{"admin_1", "shipment.admin_create", "order", "ord_p", map[string]string{"carrier": "SPX"}})
	if _, err := r.ships.GetShipmentByOrderID(context.Background(), "ord_p"); err != nil {
		t.Fatalf("shipment not created: %v", err)
	}
}

// Scenario: Order audit: Admin creates a shipment (empty carrier is the effective SPX)
func TestCreateShipmentAdminEmptyCarrierRecordsSPX(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	if _, err := r.h.CreateShipment(adminUser(), shipReq("ord_p", "")); err != nil {
		t.Fatal(err)
	}
	wantAudit(t, w, auditCall{"admin_1", "shipment.admin_create", "order", "ord_p", map[string]string{"carrier": "SPX"}})
}

// Scenario: Order audit: Long carrier name is truncated in the audit event
func TestCreateShipmentAuditCarrierTruncated(t *testing.T) {
	for name, unit := range map[string]string{"ascii": "a", "multibyte": "é"} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			r := newOrderRig(t, w)
			if _, err := r.h.CreateShipment(adminUser(), shipReq("ord_p", strings.Repeat(unit, 100))); err != nil {
				t.Fatal(err)
			}
			wantAudit(t, w, auditCall{"admin_1", "shipment.admin_create", "order", "ord_p",
				map[string]string{"carrier": strings.Repeat(unit, 64)}})
		})
	}
}

// Scenario: Order audit: Seller creates a shipment unaudited
func TestCreateShipmentSellerNotAudited(t *testing.T) {
	for name, w := range map[string]*fakeAudit{"audit down": {err: errAuditDown}, "no audit client": nil} {
		t.Run(name, func(t *testing.T) {
			r := newOrderRig(t, w)
			if _, err := r.h.CreateShipment(sellerUser(), shipReq("ord_p", "SPX")); err != nil {
				t.Fatal(err)
			}
			if w != nil && w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Shipment refused while audit is down
func TestCreateShipmentAuditDownUnavailableNoChange(t *testing.T) {
	w := &fakeAudit{err: errAuditDown}
	r := newOrderRig(t, w)
	_, err := r.h.CreateShipment(adminUser(), shipReq("ord_p", "SPX"))
	wantCode(t, err, codes.Unavailable)
	r.noShipment(t, "ord_p")
	if got := r.status(t, "ord_p"); got != repository.OrderStatusPaid {
		t.Fatalf("order = %v, want Paid", got)
	}
}

// Scenario: Order audit: Shipment refused without an audit client
func TestCreateShipmentNilAuditWriterUnavailable(t *testing.T) {
	r := newOrderRig(t, nil)
	_, err := r.h.CreateShipment(adminUser(), shipReq("ord_p", "SPX"))
	wantCode(t, err, codes.Unavailable)
	r.noShipment(t, "ord_p")
}

// Scenario: Order audit: Refused shipment calls write no event
func TestCreateShipmentRefusedWritesNoAudit(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  context.Context
		req  *orderv1.CreateShipmentRequest
		want codes.Code
	}{
		"buyer":           {buyerUser(), shipReq("ord_p", "SPX"), codes.PermissionDenied},
		"unauthenticated": {context.Background(), shipReq("ord_p", "SPX"), codes.Unauthenticated},
		"empty order id":  {adminUser(), shipReq("", "SPX"), codes.InvalidArgument},
		"unknown order":   {adminUser(), shipReq("nope", "SPX"), codes.NotFound},
	} {
		t.Run(name, func(t *testing.T) {
			w := &fakeAudit{}
			r := newOrderRig(t, w)
			_, err := r.h.CreateShipment(tc.ctx, tc.req)
			wantCode(t, err, tc.want)
			if w.n() != 0 {
				t.Fatalf("audit calls = %d, want 0", w.n())
			}
		})
	}
}

// Scenario: Order audit: Shipment on a Pending order keeps the audit row
func TestCreateShipmentAdminInvalidStatusAfterAudit(t *testing.T) {
	w := &fakeAudit{}
	r := newOrderRig(t, w)
	_, err := r.h.CreateShipment(adminUser(), shipReq("ord_pending", "SPX"))
	wantCode(t, err, codes.FailedPrecondition)
	if w.n() != 1 {
		t.Fatalf("audit calls = %d, want 1", w.n())
	}
	r.noShipment(t, "ord_pending")
}
