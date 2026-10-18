package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
)

const testPaymentDeadline = 24 * time.Hour

type placedFixture struct {
	placer repository.OrderPlacer
	orders repository.OrderRepository
	sagas  repository.SagaRepository
	// placedFacts returns the decoded OrderPlaced outbox rows keyed to orderID.
	placedFacts func(t *testing.T, orderID string) []*orderv1.OrderPlaced
}

func decodePlacedPayloads(t *testing.T, payloads [][]byte) []*orderv1.OrderPlaced {
	t.Helper()
	var out []*orderv1.OrderPlaced
	for _, p := range payloads {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(p, &env); err != nil {
			t.Fatal(err)
		}
		if env.GetType() != events.OrderPlacedEventType {
			continue
		}
		var ev orderv1.OrderPlaced
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func placedOrder(id, seller string, method paymentv1.PaymentMethod, deadline time.Duration) repository.PlacedOrder {
	o := order(id, seller)
	o.PaymentMethod = int32(method)
	return repository.PlacedOrder{Order: o, PaymentDeadline: deadline}
}

func runPlacedOutboxContract(t *testing.T, fx func(t *testing.T) placedFixture) {
	ctx := context.Background()

	// Scenario: Order expiry: Online-pay order gets a 24-hour deadline
	t.Run("TestPlaceOrdersSetsPaymentDueAtAndOrderPlaced", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la")
		id := uid("ord")
		po := placedOrder(id, "sa", paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		po.ReservationIDs = res
		if _, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{po}); err != nil {
			t.Fatal(err)
		}
		got, err := f.orders.GetOrder(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.PaymentDueAt == nil {
			t.Fatal("payment_due_at must be set for an online-pay order")
		}
		want := got.CreatedAt.Add(testPaymentDeadline)
		if d := got.PaymentDueAt.Sub(want); d < -time.Second || d > time.Second {
			t.Fatalf("payment_due_at %v, want created_at+24h %v", got.PaymentDueAt, want)
		}
		facts := f.placedFacts(t, id)
		if len(facts) != 1 {
			t.Fatalf("want exactly 1 OrderPlaced row, got %d", len(facts))
		}
		ev := facts[0]
		if ev.GetOrderId() != id || ev.GetSellerId() != "sa" || ev.GetBuyerId() != "buyer_p" || ev.GetTotalAmount() != 10 ||
			ev.GetPaymentMethod() != paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO {
			t.Fatalf("fact: %+v", ev)
		}
		if ev.GetPaymentDueAt() == nil {
			t.Fatal("OrderPlaced.payment_due_at must be set")
		}
		if d := ev.GetPaymentDueAt().AsTime().Sub(got.PaymentDueAt.UTC()); d < -time.Second || d > time.Second {
			t.Fatalf("OrderPlaced.payment_due_at %v differs from the order's %v", ev.GetPaymentDueAt().AsTime(), got.PaymentDueAt)
		}
		if ev.GetPlacedAt() == nil {
			t.Fatal("OrderPlaced.placed_at must be set")
		}
	})

	// Scenario: Order expiry: COD order has no deadline
	t.Run("TestPlaceOrdersCodHasNoPaymentDueAt", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la")
		id := uid("ord")
		// The service leaves PaymentDeadline zero for COD; the order must carry none.
		po := placedOrder(id, "sa", paymentv1.PaymentMethod_PAYMENT_METHOD_COD, 0)
		po.ReservationIDs = res
		if _, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{po}); err != nil {
			t.Fatal(err)
		}
		got, err := f.orders.GetOrder(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.PaymentDueAt != nil {
			t.Fatalf("COD order must have no payment_due_at, got %v", got.PaymentDueAt)
		}
		facts := f.placedFacts(t, id)
		if len(facts) != 1 {
			t.Fatalf("COD order must still emit one OrderPlaced, got %d", len(facts))
		}
		if facts[0].GetPaymentDueAt() != nil {
			t.Fatalf("OrderPlaced.payment_due_at must be unset for COD, got %v", facts[0].GetPaymentDueAt())
		}
		if facts[0].GetPaymentMethod() != paymentv1.PaymentMethod_PAYMENT_METHOD_COD {
			t.Fatalf("payment_method %v", facts[0].GetPaymentMethod())
		}
	})

	// Scenario: Order expiry: Rolled-back placement emits no OrderPlaced
	t.Run("TestPlaceOrdersRollbackWritesNoOrderPlaced", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la", "lb")
		if err := f.sagas.UpdateReservationStatus(ctx, res[1], repository.ReservationStatusReleased); err != nil {
			t.Fatal(err)
		}
		oa, ob := uid("ord"), uid("ord")
		a := placedOrder(oa, "sa", paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		a.ReservationIDs = res[:1]
		b := placedOrder(ob, "sb", paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		b.ReservationIDs = res[1:]
		_, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{a, b})
		if !errors.Is(err, repository.ErrReservationLost) {
			t.Fatalf("want ErrReservationLost, got %v", err)
		}
		for _, id := range []string{oa, ob} {
			if _, err := f.orders.GetOrder(ctx, id); !errors.Is(err, repository.ErrOrderNotFound) {
				t.Fatalf("order %s must not exist: %v", id, err)
			}
			if n := len(f.placedFacts(t, id)); n != 0 {
				t.Fatalf("rolled-back placement wrote %d OrderPlaced rows for %s", n, id)
			}
		}
	})

	// Scenario: Order expiry: Each seller order of one checkout gets its own OrderPlaced
	t.Run("one OrderPlaced per order of a multi-order placement", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la", "lb")
		oa, ob := uid("ord"), uid("ord")
		a := placedOrder(oa, "sa", paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		a.ReservationIDs = res[:1]
		b := placedOrder(ob, "sb", paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		b.ReservationIDs = res[1:]
		if _, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{a, b}); err != nil {
			t.Fatal(err)
		}
		for id, seller := range map[string]string{oa: "sa", ob: "sb"} {
			facts := f.placedFacts(t, id)
			if len(facts) != 1 || facts[0].GetSellerId() != seller || facts[0].GetOrderId() != id {
				t.Fatalf("order %s: want one OrderPlaced for seller %s, got %+v", id, seller, facts)
			}
		}
	})
}

func TestPlacedOutbox_InMemory(t *testing.T) {
	runPlacedOutboxContract(t, func(t *testing.T) placedFixture {
		outbox := repository.NewInMemoryOutboxRepository()
		orders := repository.NewInMemoryOrderRepository(repository.WithPlacedOutbox(events.BuildPlacedOutboxRow),
			repository.WithInMemoryOutbox(outbox))
		sagas := repository.NewInMemorySagaRepository()
		return placedFixture{
			placer: repository.NewInMemoryOrderPlacer(orders, sagas), orders: orders, sagas: sagas,
			placedFacts: func(t *testing.T, orderID string) []*orderv1.OrderPlaced {
				var payloads [][]byte
				for _, r := range outbox.EnqueuedRows() {
					if r.AggregateID == orderID {
						payloads = append(payloads, r.Payload)
					}
				}
				return decodePlacedPayloads(t, payloads)
			},
		}
	})
}

func TestPlacedOutbox_Postgres(t *testing.T) {
	pool := pgPool(t)
	runPlacedOutboxContract(t, func(t *testing.T) placedFixture {
		orders := repository.NewPostgresOrderRepository(pool, repository.WithPlacedOutbox(events.BuildPlacedOutboxRow))
		return placedFixture{
			placer: orders, orders: orders, sagas: repository.NewPostgresSagaRepository(pool),
			placedFacts: func(t *testing.T, orderID string) []*orderv1.OrderPlaced {
				r, err := pool.Query(context.Background(), `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, orderID)
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				var payloads [][]byte
				for r.Next() {
					var p []byte
					if err := r.Scan(&p); err != nil {
						t.Fatal(err)
					}
					payloads = append(payloads, p)
				}
				if err := r.Err(); err != nil {
					t.Fatal(err)
				}
				return decodePlacedPayloads(t, payloads)
			},
		}
	})
}
