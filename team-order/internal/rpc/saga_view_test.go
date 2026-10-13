package rpc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type sagaRig struct {
	domain *upstreamtest.Domain
	orders *store.InMemoryOrderRepository
	svc    *order.Service
	h      *rpc.OrderHandler
	order  store.Order
}

// newSagaRig places a real order (quantity 2 of lst_1, stock 10 -> 8).
func newSagaRig(t *testing.T) *sagaRig {
	t.Helper()
	r := &sagaRig{domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10}), orders: store.NewInMemoryOrderRepository()}
	carts := store.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "seller_1"}); err != nil {
		t.Fatal(err)
	}
	r.svc = order.NewOrderService(r.orders, carts, nil, nil, r.domain, nil, nil,
		order.WithReleaseRetry(time.Second, 1, time.Millisecond))
	r.h = rpc.NewOrderHandler(r.svc, nil, nil)
	placed, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", store.Address{}, nil, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	r.order = placed[0]
	return r
}

func (r *sagaRig) view(t *testing.T) *orderv1.GetSagaStateResponse {
	t.Helper()
	res, err := r.h.GetSagaState(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.GetSagaStateRequest{OrderId: r.order.ID})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func steps(res *orderv1.GetSagaStateResponse) map[string]*orderv1.SagaStep {
	out := map[string]*orderv1.SagaStep{}
	for _, s := range res.GetSteps() {
		switch {
		case strings.Contains(s.GetName(), "Order Created"):
			out["created"] = s
		case strings.Contains(s.GetName(), "Stock Reserved"):
			out["reserved"] = s
		case strings.Contains(s.GetName(), "Payment"):
			out["payment"] = s
		case strings.Contains(s.GetName(), "Order Confirmed"):
			out["confirmation"] = s
		case strings.Contains(s.GetName(), "Compensation"):
			out["compensation"] = s
		}
	}
	return out
}

func TestSagaView_PendingOrder(t *testing.T) {
	r := newSagaRig(t)
	res := r.view(t)
	st := steps(res)
	if st["created"].GetStatus() != "SUCCESS" || st["reserved"].GetStatus() != "SUCCESS" ||
		st["payment"].GetStatus() != "PENDING" || st["confirmation"].GetStatus() != "PENDING" || res.GetIsCompensated() {
		t.Fatalf("pending view wrong: %v", res)
	}
	if st["payment"].GetTimestamp() != nil || st["confirmation"].GetTimestamp() != nil {
		t.Fatal("unrecorded steps must carry no timestamp")
	}
	if st["created"].GetTimestamp() == nil || st["reserved"].GetTimestamp() == nil {
		t.Fatal("created and reserved times are recorded")
	}
}

func TestSagaView_PaidOrderShowsPaymentTime(t *testing.T) {
	r := newSagaRig(t)
	paid, err := r.orders.UpdateOrderStatusFrom(context.Background(), r.order.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, "")
	if err != nil {
		t.Fatal(err)
	}
	res := r.view(t)
	st := steps(res)
	if st["payment"].GetStatus() != "SUCCESS" || !st["payment"].GetTimestamp().AsTime().Equal(*paid.PaidAt) {
		t.Fatalf("payment step: %v", st["payment"])
	}
	if res.GetIsCompensated() || st["compensation"] != nil {
		t.Fatal("no compensation for a paid order")
	}
}

func TestSagaView_CancelledAllReleased(t *testing.T) {
	r := newSagaRig(t)
	if _, err := r.svc.CancelOrder(context.Background(), r.order.ID); err != nil {
		t.Fatal(err)
	}
	res := r.view(t)
	st := steps(res)
	if st["payment"].GetStatus() != "SKIPPED" || st["compensation"].GetStatus() != "COMPENSATED" || !res.GetIsCompensated() {
		t.Fatalf("cancelled view wrong: %v", res)
	}
	if res.GetCompensationReason() != "order cancelled" {
		t.Fatalf("reason %q", res.GetCompensationReason())
	}
}

func TestSagaView_CancelledWithParkedRelease(t *testing.T) {
	r := newSagaRig(t)
	r.domain.ReleaseErr = func(string) error { return status.Error(codes.Unavailable, "down") }
	if _, err := r.svc.CancelOrder(context.Background(), r.order.ID); err != nil {
		t.Fatal(err)
	}
	res := r.view(t)
	if st := steps(res); st["compensation"].GetStatus() != "PENDING" || res.GetIsCompensated() {
		t.Fatalf("parked release must show compensation PENDING: %v", res)
	}
}

func TestForceFailSaga_UnknownStepRejectedBeforeAnyWrite(t *testing.T) {
	r := newSagaRig(t)
	_, err := r.h.ForceFailSaga(adminCtx(), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID, FailStep: "banana"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if o, _ := r.orders.GetOrder(context.Background(), r.order.ID); o.Status != store.OrderStatusPending || r.domain.Stock("lst_1") != 8 {
		t.Fatalf("order must be untouched: %v stock %d", o.Status, r.domain.Stock("lst_1"))
	}
}

func TestForceFailSaga_CleanFailReportsSuccess(t *testing.T) {
	r := newSagaRig(t)
	res, err := r.h.ForceFailSaga(adminCtx(), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID, FailStep: "payment"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetSuccess() || !res.GetSagaState().GetIsCompensated() || r.domain.Stock("lst_1") != 10 {
		t.Fatalf("clean force-fail: %v stock %d", res, r.domain.Stock("lst_1"))
	}
	if o, _ := r.orders.GetOrder(context.Background(), r.order.ID); o.Status != store.OrderStatusCancelled {
		t.Fatalf("order %v", o.Status)
	}
}

func TestForceFailSaga_ParkedReleaseReportsFailure(t *testing.T) {
	r := newSagaRig(t)
	r.domain.ReleaseErr = func(string) error { return status.Error(codes.Unavailable, "down") }
	res, err := r.h.ForceFailSaga(adminCtx(), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetSuccess() || !strings.Contains(res.GetMessage(), "pending retry") {
		t.Fatalf("want success=false naming the pending release: %v", res)
	}
	if o, _ := r.orders.GetOrder(context.Background(), r.order.ID); o.Status != store.OrderStatusCancelled {
		t.Fatalf("order %v", o.Status)
	}
}

func TestForceFailSaga_ShippedOrderRefused(t *testing.T) {
	r := newSagaRig(t)
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), r.order.ID, store.OrderStatusShipped, []store.OrderStatus{store.OrderStatusPending}, "T"); err != nil {
		t.Fatal(err)
	}
	_, err := r.h.ForceFailSaga(adminCtx(), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID, FailStep: "shipping"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if o, _ := r.orders.GetOrder(context.Background(), r.order.ID); o.Status != store.OrderStatusShipped {
		t.Fatalf("order %v", o.Status)
	}
}

func TestSagaAccess_BuyerOrAdminOnly(t *testing.T) {
	r := newSagaRig(t)
	admin := auth.WithPrincipal(context.Background(), auth.Principal{ID: "admin_1", Type: "user", Scopes: []string{"admin", "order.admin"}})
	if _, err := r.h.GetSagaState(admin, &orderv1.GetSagaStateRequest{OrderId: r.order.ID}); err != nil {
		t.Fatalf("admin: %v", err)
	}
	for _, who := range []string{"seller_1", "stranger"} {
		if _, err := r.h.GetSagaState(incomingPrincipalCtx(who, "x"), &orderv1.GetSagaStateRequest{OrderId: r.order.ID}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s: want PermissionDenied, got %v", who, err)
		}
		if _, err := r.h.ForceFailSaga(incomingPrincipalCtx(who, "x"), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s force-fail: want PermissionDenied, got %v", who, err)
		}
	}
}

// ForceFailSaga cancels through the same claim as CancelOrder, so it writes the
// OrderCancelled fact once, with the status the order was cancelled from.
func TestForceFailSaga_EmitsCancelledFactThroughTheClaim(t *testing.T) {
	outbox := store.NewInMemoryOutboxRepository()
	orders := store.NewInMemoryOrderRepository(store.WithPaidOutbox(events.BuildPaidOutboxRow),
		store.WithCancelledOutbox(events.BuildCancelledOutboxRow), store.WithInMemoryOutbox(outbox))
	carts := store.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "seller_1"}); err != nil {
		t.Fatal(err)
	}
	svc := order.NewOrderService(orders, carts, nil, nil, upstreamtest.NewDomain(map[string]int32{"lst_1": 10}), nil, nil,
		order.WithReleaseRetry(time.Second, 1, time.Millisecond))
	h := rpc.NewOrderHandler(svc, nil, nil)
	placed, err := svc.CreateOrdersFromCart(context.Background(), "buyer_1", store.Address{}, nil, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	o := placed[0]
	if _, err := orders.UpdateOrderStatusFrom(context.Background(), o.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	ctx := adminCtx()
	if _, err := h.ForceFailSaga(ctx, &orderv1.ForceFailSagaRequest{OrderId: o.ID, FailStep: "shipping"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ForceFailSaga(ctx, &orderv1.ForceFailSagaRequest{OrderId: o.ID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("second force-fail: want FailedPrecondition, got %v", err)
	}
	var facts []*orderv1.OrderCancelled
	for _, row := range outbox.EnqueuedRows() {
		if row.AggregateID != o.ID || row.EventType != events.OrderCancelledEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderCancelled
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		facts = append(facts, &ev)
	}
	if len(facts) != 1 || facts[0].GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PAID || facts[0].GetSellerId() != "seller_1" {
		t.Fatalf("want one OrderCancelled from PAID, got %+v", facts)
	}
}
