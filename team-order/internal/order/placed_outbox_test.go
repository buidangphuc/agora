package order_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

func placedFacts(t *testing.T, outbox *pgOutbox) map[string]*orderv1.OrderPlaced {
	t.Helper()
	out := map[string]*orderv1.OrderPlaced{}
	for _, r := range outbox.EnqueuedRows() {
		if r.EventType != OrderPlacedEventType {
			continue
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var ev orderv1.OrderPlaced
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		if _, dup := out[ev.GetOrderId()]; dup {
			t.Fatalf("two OrderPlaced rows for order %s", ev.GetOrderId())
		}
		out[ev.GetOrderId()] = &ev
	}
	return out
}

// placedRig is the two-seller checkout rig with the OrderPlaced outbox wired.
func placedRig(t *testing.T, deadline time.Duration) (*checkoutRig, *pgOutbox) {
	t.Helper()
	outbox := newPGOutbox(t)
	r := &checkoutRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_a": 10, "lst_b": 5}),
		orders: outbox.orders(store.WithPlacedOutbox(BuildPlacedOutboxRow)),
		sagas:  &recordingSagaRepo{SagaRepository: outbox.sagas()},
		cart: &fakeCartRepo{items: []store.CartItem{
			{ID: "ci_b", ListingID: "lst_b", Quantity: 1, SellerID: "sb", UnitPrice: 2000},
			{ID: "ci_a", ListingID: "lst_a", Quantity: 2, SellerID: "sa", UnitPrice: 1000},
		}},
	}
	r.svc = newSvc(t, r.orders, r.cart, nil, nil, r.domain, nil, nil,
		order.WithSagaRepository(r.sagas),
		order.WithPaymentDeadline(deadline))
	return r, outbox
}

// Scenario: Order expiry: Each seller order of one checkout gets its own OrderPlaced
func TestCheckoutEmitsOrderPlacedPerOrder(t *testing.T) {
	r, outbox := placedRig(t, 24*time.Hour)
	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil,
		int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), "")
	if err != nil || len(got) != 2 {
		t.Fatalf("checkout: %v %d", err, len(got))
	}
	facts := placedFacts(t, outbox)
	if len(facts) != 2 {
		t.Fatalf("want exactly 2 OrderPlaced rows, got %d", len(facts))
	}
	for _, o := range got {
		ev, ok := facts[o.ID]
		if !ok {
			t.Fatalf("no OrderPlaced for order %s", o.ID)
		}
		if ev.GetSellerId() != o.SellerID || ev.GetTotalAmount() != o.TotalAmount || ev.GetBuyerId() != "buyer_1" {
			t.Fatalf("order %s fact %+v does not carry the order's own seller/total", o.ID, ev)
		}
		if ev.GetPaymentDueAt() == nil {
			t.Fatalf("online order %s: OrderPlaced.payment_due_at unset", o.ID)
		}
	}
}

// Scenario: Order expiry: Online-pay order gets a 24-hour deadline (service wires
// the configured deadline into placement for every non-COD order).
func TestCheckoutOnlineOrdersGetDeadlineFromConfig(t *testing.T) {
	r, _ := placedRig(t, 24*time.Hour)
	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil,
		int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got {
		stored, err := r.orders.GetOrder(context.Background(), o.ID)
		if err != nil || stored.PaymentDueAt == nil {
			t.Fatalf("order %s: %v due=%v", o.ID, err, stored.PaymentDueAt)
		}
		if d := stored.PaymentDueAt.Sub(stored.CreatedAt.Add(24 * time.Hour)); d < -time.Second || d > time.Second {
			t.Fatalf("order %s payment_due_at %v, want created_at+24h", o.ID, stored.PaymentDueAt)
		}
	}
}

// Scenario: Order expiry: COD order has no deadline
func TestCheckoutCodOrdersHaveNoDeadline(t *testing.T) {
	r, outbox := placedRig(t, 24*time.Hour)
	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil,
		int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD), "")
	if err != nil || len(got) != 2 {
		t.Fatalf("checkout: %v", err)
	}
	facts := placedFacts(t, outbox)
	for _, o := range got {
		stored, _ := r.orders.GetOrder(context.Background(), o.ID)
		if stored.PaymentDueAt != nil {
			t.Fatalf("COD order %s must have no payment_due_at, got %v", o.ID, stored.PaymentDueAt)
		}
		if ev, ok := facts[o.ID]; !ok || ev.GetPaymentDueAt() != nil {
			t.Fatalf("COD order %s: want an OrderPlaced with no payment_due_at, got %+v", o.ID, ev)
		}
	}
}

// Scenario: Order expiry: A later config change does not move an existing deadline
func TestPaymentDueAtSnapshotSurvivesConfigChange(t *testing.T) {
	r, _ := placedRig(t, 24*time.Hour)
	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil,
		int32(paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO), "")
	if err != nil || len(got) == 0 {
		t.Fatalf("checkout: %v", err)
	}
	before, err := r.orders.GetOrder(context.Background(), got[0].ID)
	if err != nil || before.PaymentDueAt == nil {
		t.Fatalf("order: %v %v", err, before.PaymentDueAt)
	}
	// "Restart" with PAYMENT_DEADLINE=1h over the same store.
	restarted := newSvc(t, r.orders, r.cart, nil, nil, r.domain, nil, nil,
		order.WithPaymentDeadline(time.Hour))
	after, err := restarted.GetOrder(context.Background(), before.ID)
	if err != nil || after.PaymentDueAt == nil {
		t.Fatalf("get after restart: %v %v", err, after.PaymentDueAt)
	}
	if !after.PaymentDueAt.Equal(*before.PaymentDueAt) {
		t.Fatalf("deadline moved from %v to %v after a config change", before.PaymentDueAt, after.PaymentDueAt)
	}
}
