package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type reasonRig struct {
	domain *upstreamtest.Domain
	orders *store.PostgresOrderRepository
	outbox *pgOutbox
	h      *rpc.OrderHandler
	order  store.Order
}

// newReasonRig places a real order (quantity 2 of lst_1, stock 10 -> 8) with the
// OrderCancelled outbox on, so stock, row and event can all be observed.
func newReasonRig(t *testing.T) *reasonRig {
	t.Helper()
	r := &reasonRig{domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10}), outbox: newPGOutbox(t)}
	r.orders = r.outbox.orders(store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithCancelledOutbox(BuildCancelledOutboxRow))
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "seller_1"}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, r.orders, carts, nil, nil, r.domain, nil, nil,
		order.WithSagaRepository(r.outbox.sagas()), order.WithReleaseRetry(time.Second, 1, time.Millisecond))
	r.h = newHandler(t, svc, nil, nil, withAudit(&fakeAudit{}))
	placed, err := svc.CreateOrdersFromCart(context.Background(), "buyer_1", store.Address{}, nil, 2, "")
	if err != nil || len(placed) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	r.order = placed[0]
	if r.domain.Stock("lst_1") != 8 {
		t.Fatalf("setup stock %d", r.domain.Stock("lst_1"))
	}
	return r
}

func (r *reasonRig) facts(t *testing.T) []*orderv1.OrderCancelled {
	t.Helper()
	var out []*orderv1.OrderCancelled
	for _, row := range r.outbox.EnqueuedRows() {
		if row.AggregateID != r.order.ID || row.EventType != OrderCancelledEventType {
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
		out = append(out, &ev)
	}
	return out
}

func (r *reasonRig) wireOrder(t *testing.T) *orderv1.Order {
	t.Helper()
	res, err := r.h.GetOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.GetOrderRequest{Id: r.order.ID})
	if err != nil {
		t.Fatal(err)
	}
	return res.GetOrder()
}

func (r *reasonRig) assertUntouched(t *testing.T) {
	t.Helper()
	if o := r.wireOrder(t); o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_PENDING || o.GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		t.Fatalf("order must be unchanged, got %v / %v", o.GetStatus(), o.GetCancelReason())
	}
	if n := len(r.facts(t)); n != 0 {
		t.Fatalf("no OrderCancelled expected, got %d", n)
	}
	if r.domain.Stock("lst_1") != 8 {
		t.Fatalf("stock must not be released, got %d", r.domain.Stock("lst_1"))
	}
}

// Scenario "Order expiry: Buyer cancel stores the chosen reason" (handler half).
func TestCancelOrderStoresChosenReason(t *testing.T) {
	r := newReasonRig(t)
	res, err := r.h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CancelOrderRequest{Id: r.order.ID, ReasonCode: orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetOrder().GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND {
		t.Fatalf("response cancel_reason %v", res.GetOrder().GetCancelReason())
	}
	if o := r.wireOrder(t); o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_CANCELLED || o.GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND {
		t.Fatalf("GetOrder: %v / %v", o.GetStatus(), o.GetCancelReason())
	}
	if f := r.facts(t); len(f) != 1 || f[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND {
		t.Fatalf("facts: %+v", f)
	}
}

// Scenario "Order expiry: Cancel without a reason code still succeeds".
func TestCancelOrderWithoutReasonCode(t *testing.T) {
	r := newReasonRig(t)
	if _, err := r.h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CancelOrderRequest{Id: r.order.ID}); err != nil {
		t.Fatal(err)
	}
	if o := r.wireOrder(t); o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_CANCELLED || o.GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		t.Fatalf("GetOrder: %v / %v", o.GetStatus(), o.GetCancelReason())
	}
	if r.domain.Stock("lst_1") != 10 {
		t.Fatalf("stock must be released, got %d", r.domain.Stock("lst_1"))
	}
	if f := r.facts(t); len(f) != 1 || f[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		t.Fatalf("facts: %+v", f)
	}
}

// Scenario "Order expiry: Buyer cannot send a system reason code".
func TestCancelOrderSystemReasonCodeDenied(t *testing.T) {
	for _, code := range []orderv1.CancelReason{
		orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED,
		orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL,
	} {
		t.Run(code.String(), func(t *testing.T) {
			r := newReasonRig(t)
			_, err := r.h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CancelOrderRequest{Id: r.order.ID, ReasonCode: code})
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("want PermissionDenied, got %v", err)
			}
			r.assertUntouched(t)
		})
	}
}

// Scenario "Order expiry: Another user cannot cancel with a reason".
func TestCancelOrderOtherUserWithReasonDenied(t *testing.T) {
	r := newReasonRig(t)
	_, err := r.h.CancelOrder(incomingPrincipalCtx("buyer_2", "buyer"),
		&orderv1.CancelOrderRequest{Id: r.order.ID, ReasonCode: orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	r.assertUntouched(t)
}

// Scenario "Order expiry: Cancel of a shipped order is refused" (handler half).
func TestCancelOrderShippedWithReasonRefused(t *testing.T) {
	r := newReasonRig(t)
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), r.order.ID, store.OrderStatusShipped,
		[]store.OrderStatus{store.OrderStatusPending}, "TRK"); err != nil {
		t.Fatal(err)
	}
	_, err := r.h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"),
		&orderv1.CancelOrderRequest{Id: r.order.ID, ReasonCode: orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if o := r.wireOrder(t); o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_SHIPPED || o.GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
		t.Fatalf("order must be untouched: %v / %v", o.GetStatus(), o.GetCancelReason())
	}
}

// Scenario "Order expiry: Admin force-fail records its own reason".
func TestForceFailSagaStoresAdminReason(t *testing.T) {
	r := newReasonRig(t)
	if _, err := r.h.ForceFailSaga(scopedUser("admin_1", "admin", "order.admin"), &orderv1.ForceFailSagaRequest{OrderId: r.order.ID}); err != nil {
		t.Fatal(err)
	}
	if o := r.wireOrder(t); o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_CANCELLED || o.GetCancelReason() != orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL {
		t.Fatalf("GetOrder: %v / %v", o.GetStatus(), o.GetCancelReason())
	}
	if f := r.facts(t); len(f) != 1 || f[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL {
		t.Fatalf("facts: %+v", f)
	}
}

// Scenario "Order expiry: A seller or admin cannot request Cancelled through
// UpdateOrderStatus" (handler half, seller and admin classes).
func TestUpdateOrderStatusCancelledDeniedOverWire(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"seller": incomingPrincipalCtx("seller_1", "seller"),
		"admin":  scopedUser("admin_1", "admin", "order.admin"),
	} {
		t.Run(name, func(t *testing.T) {
			r := newReasonRig(t)
			_, err := r.h.UpdateOrderStatus(ctx, &orderv1.UpdateOrderStatusRequest{Id: r.order.ID, Status: orderv1.OrderStatus_ORDER_STATUS_CANCELLED})
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("want PermissionDenied, got %v", err)
			}
			r.assertUntouched(t)
		})
	}
}
