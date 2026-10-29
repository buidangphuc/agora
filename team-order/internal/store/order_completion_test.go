package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

// Task 4 (order-receipt-completion). The repository API these tests pin:
//   - store.WithAutoCompleteAfter / WithReturnWindow / WithShipByAfter (OrderRepoOption):
//     durations in force at the moment of each stamp.
//   - store.WithCompletedOutbox(b) (OrderRepoOption) and store.CompletedOutboxBuilder.
//   - store.WithCompletedBy(int32) (StatusOption): the orderv1.CompletedBy of the write.
//   - Order gains ShippedAt, AutoCompleteAt, CompletedAt, ReturnWindowEndsAt, ShipBy (*time.Time)
//     and CompletedBy (int32, set on the row a won move to Completed returns).
//   - BuildCompletedOutboxRow, OrderCompletedEventType.

const (
	rcAutoComplete = 7 * 24 * time.Hour
	rcReturnWindow = 15 * 24 * time.Hour
	rcShipBy       = 2 * 24 * time.Hour
)

var (
	rcFromShipped    = []store.OrderStatus{store.OrderStatusShipped}
	rcFromPaidOrPend = []store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}
	rcFromPaid       = []store.OrderStatus{store.OrderStatusPaid}
)

type rcFixture struct {
	repo   store.OrderRepository
	placer store.OrderPlacer
	sagas  store.SagaRepository
	// rows returns the decoded envelopes written for an order, in write order.
	rows func(t *testing.T, orderID string) []*eventsv1.EventEnvelope
}

type rcFactory func(t *testing.T, extra ...store.OrderRepoOption) rcFixture

func rcBaseOpts() []store.OrderRepoOption {
	return []store.OrderRepoOption{
		store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithPlacedOutbox(BuildPlacedOutboxRow),
		store.WithCompletedOutbox(BuildCompletedOutboxRow),
		store.WithAutoCompleteAfter(rcAutoComplete),
		store.WithReturnWindow(rcReturnWindow),
		store.WithShipByAfter(rcShipBy),
	}
}

func rcDecode(t *testing.T, payloads [][]byte) []*eventsv1.EventEnvelope {
	t.Helper()
	var out []*eventsv1.EventEnvelope
	for _, p := range payloads {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(p, &env); err != nil {
			t.Fatal(err)
		}
		out = append(out, &env)
	}
	return out
}

func rcCompleted(t *testing.T, envs []*eventsv1.EventEnvelope) []*orderv1.OrderCompleted {
	t.Helper()
	var out []*orderv1.OrderCompleted
	for _, e := range envs {
		if e.GetType() != OrderCompletedEventType {
			continue
		}
		var ev orderv1.OrderCompleted
		if err := proto.Unmarshal(e.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func rcNewOrder(ctx context.Context, t *testing.T, repo store.OrderRepository, method paymentv1.PaymentMethod) store.Order {
	t.Helper()
	o, err := repo.CreateOrder(ctx, store.Order{
		BuyerID: "b", SellerID: "s", TotalAmount: 120000,
		Currency: "VND", PaymentMethod: int32(method),
		Items: []store.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: 120000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func rcShip(ctx context.Context, t *testing.T, repo store.OrderRepository, o store.Order) store.Order {
	t.Helper()
	if o.PaymentMethod != int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD) {
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, fromPending, ""); err != nil {
			t.Fatal(err)
		}
	}
	shipped, err := repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, rcFromPaidOrPend, "TRK-"+o.ID)
	if err != nil {
		t.Fatal(err)
	}
	return shipped
}

func rcNear(t *testing.T, name string, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %v", name, want)
	}
	if d := got.Sub(want); d < -time.Second || d > time.Second {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func runCompletionContract(t *testing.T, fx rcFactory) {
	ctx := context.Background()
	buyer := store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_BUYER))
	seller := store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_SELLER))

	// Scenario: Order receipt: Shipping stamps the auto-complete deadline
	t.Run("TestShippingStampsAutoCompleteAt", func(t *testing.T) {
		f := fx(t)
		o := rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
		shipped := rcShip(ctx, t, f.repo, o)
		if shipped.Status != store.OrderStatusShipped || shipped.ShippedAt == nil || shipped.AutoCompleteAt == nil {
			t.Fatalf("shipped: %+v", shipped)
		}
		if want := shipped.ShippedAt.Add(rcAutoComplete); !shipped.AutoCompleteAt.Equal(want) {
			t.Fatalf("auto_complete_at %v, want shipped_at+7d %v", shipped.AutoCompleteAt, want)
		}
		got, err := f.repo.GetOrder(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ShippedAt == nil || got.AutoCompleteAt == nil || got.CompletedAt != nil || got.ReturnWindowEndsAt != nil {
			t.Fatalf("persisted stamps: %+v", got)
		}
		if d := got.AutoCompleteAt.Sub(got.ShippedAt.Add(rcAutoComplete)); d < -time.Millisecond || d > time.Millisecond {
			t.Fatalf("stored auto_complete_at %v is not shipped_at+7d", got.AutoCompleteAt)
		}
	})

	// Scenario: Order receipt: Seller completion clears the auto-complete deadline
	t.Run("TestCompletionClearsAutoCompleteAt", func(t *testing.T) {
		f := fx(t)
		o := rcShip(ctx, t, f.repo, rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO))
		done, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", seller)
		if err != nil {
			t.Fatal(err)
		}
		if done.CompletedAt == nil || done.AutoCompleteAt != nil {
			t.Fatalf("returned row: %+v", done)
		}
		got, _ := f.repo.GetOrder(ctx, o.ID)
		if got.Status != store.OrderStatusCompleted || got.CompletedAt == nil || got.AutoCompleteAt != nil {
			t.Fatalf("stored row: completed_at=%v auto_complete_at=%v status=%v", got.CompletedAt, got.AutoCompleteAt, got.Status)
		}
		rcNear(t, "return_window_ends_at", got.ReturnWindowEndsAt, got.CompletedAt.Add(rcReturnWindow))
	})

	// Scenario: Order receipt: Buyer confirm emits OrderCompleted with completed_by BUYER
	t.Run("TestBuyerCompletionEmitsOrderCompleted", func(t *testing.T) {
		f := fx(t)
		o := rcShip(ctx, t, f.repo, rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO))
		done, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", buyer)
		if err != nil {
			t.Fatal(err)
		}
		got := rcCompleted(t, f.rows(t, o.ID))
		if len(got) != 1 {
			t.Fatalf("want exactly 1 OrderCompleted, got %d", len(got))
		}
		ev := got[0]
		if ev.GetOrderId() != o.ID || ev.GetBuyerId() != "b" || ev.GetSellerId() != "s" ||
			ev.GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_BUYER || !ev.GetPaidOnline() ||
			ev.GetTotalAmount() != 120000 || ev.GetCurrency() != "VND" {
			t.Fatalf("fact: %+v", ev)
		}
		if ev.GetCompletedAt() == nil || done.CompletedAt == nil ||
			ev.GetCompletedAt().AsTime().Sub(done.CompletedAt.UTC()).Abs() > time.Millisecond {
			t.Fatalf("completed_at %v vs order %v", ev.GetCompletedAt(), done.CompletedAt)
		}
		if ev.GetReturnWindowEndsAt() == nil || done.ReturnWindowEndsAt == nil ||
			ev.GetReturnWindowEndsAt().AsTime().Sub(done.ReturnWindowEndsAt.UTC()).Abs() > time.Millisecond {
			t.Fatalf("return_window_ends_at %v vs order %v", ev.GetReturnWindowEndsAt(), done.ReturnWindowEndsAt)
		}
		var id string
		for _, e := range f.rows(t, o.ID) {
			if e.GetType() == OrderCompletedEventType {
				id = e.GetEventId()
			}
		}
		if id != "order-completed:"+o.ID {
			t.Fatalf("event id %q", id)
		}
	})

	// Scenario: Order receipt: Seller completion still works and emits OrderCompleted
	t.Run("TestSellerCompletionEmitsOrderCompleted", func(t *testing.T) {
		f := fx(t)
		o := rcShip(ctx, t, f.repo, rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO))
		if _, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", seller); err != nil {
			t.Fatal(err)
		}
		got := rcCompleted(t, f.rows(t, o.ID))
		if len(got) != 1 || got[0].GetCompletedBy() != orderv1.CompletedBy_COMPLETED_BY_SELLER {
			t.Fatalf("want 1 OrderCompleted by SELLER, got %+v", got)
		}
		// A second completion loses the CAS and emits nothing more.
		if _, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", buyer); !errors.Is(err, store.ErrStatusConflict) {
			t.Fatalf("second completion: want ErrStatusConflict, got %v", err)
		}
		if n := len(rcCompleted(t, f.rows(t, o.ID))); n != 1 {
			t.Fatalf("a lost completion must not emit: %d rows", n)
		}
	})

	// Scenario: Order receipt: A cash-on-delivery completion is marked not paid online
	t.Run("TestOrderCompletedPaidOnlineFalseForCOD", func(t *testing.T) {
		f := fx(t)
		o := rcShip(ctx, t, f.repo, rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_COD))
		if _, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", buyer); err != nil {
			t.Fatal(err)
		}
		got := rcCompleted(t, f.rows(t, o.ID))
		if len(got) != 1 || got[0].GetPaidOnline() {
			t.Fatalf("want 1 OrderCompleted with paid_online=false, got %+v", got)
		}
	})

	// Scenario: Order receipt: Payment stamps ship_by and the paid event carries it
	t.Run("TestPaymentStampsShipByAndPaidEventCarriesIt", func(t *testing.T) {
		f := fx(t)
		o := rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
		if o.ShipBy != nil {
			t.Fatalf("pending online order must have no ship_by: %v", o.ShipBy)
		}
		paid, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, fromPending, "")
		if err != nil || paid.PaidAt == nil {
			t.Fatalf("pay: %v %+v", err, paid)
		}
		rcNear(t, "returned ship_by", paid.ShipBy, paid.PaidAt.Add(rcShipBy))
		got, _ := f.repo.GetOrder(ctx, o.ID)
		rcNear(t, "stored ship_by", got.ShipBy, got.PaidAt.Add(rcShipBy))

		var ev orderv1.OrderPaidEvent
		n := 0
		for _, e := range f.rows(t, o.ID) {
			if e.GetType() == OrderPaidEventType {
				n++
				if err := proto.Unmarshal(e.GetPayload(), &ev); err != nil {
					t.Fatal(err)
				}
			}
		}
		if n != 1 || ev.GetSellerId() != "s" || ev.GetShipBy() == nil ||
			ev.GetShipBy().AsTime().Sub(got.ShipBy.UTC()).Abs() > time.Millisecond {
			t.Fatalf("paid event: n=%d %+v", n, &ev)
		}
	})

	// Scenario: Order receipt: A changed config does not move an existing ship_by
	// (a stamped ship_by survives every later write of the same order).
	t.Run("TestShipByIsSnapshottedAtPayment", func(t *testing.T) {
		f := fx(t)
		o := rcNewOrder(ctx, t, f.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
		paid, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, fromPending, "")
		if err != nil {
			t.Fatal(err)
		}
		orig := *paid.ShipBy
		if _, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, rcFromPaid, "T"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "", buyer); err != nil {
			t.Fatal(err)
		}
		got, _ := f.repo.GetOrder(ctx, o.ID)
		if got.ShipBy == nil || got.ShipBy.Sub(orig).Abs() > time.Millisecond {
			t.Fatalf("ship_by moved: %v -> %v", orig, got.ShipBy)
		}
	})
}

// Scenario: Order receipt: Outbox failure rolls the completion back
func runCompletionRollbackContract(t *testing.T, mk func(t *testing.T, opts ...store.OrderRepoOption) store.OrderRepository) {
	ctx := context.Background()
	failing := mk(t, store.WithAutoCompleteAfter(rcAutoComplete), store.WithReturnWindow(rcReturnWindow),
		store.WithCompletedOutbox(func(store.Order) (outbox.Row, error) {
			return outbox.Row{}, errors.New("boom")
		}))
	o := rcShip(ctx, t, failing, rcNewOrder(ctx, t, failing, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO))
	_, err := failing.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusCompleted, rcFromShipped, "",
		store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_BUYER)))
	if err == nil {
		t.Fatal("completion must fail when the outbox row cannot be written")
	}
	got, gerr := failing.GetOrder(ctx, o.ID)
	if gerr != nil {
		t.Fatal(gerr)
	}
	if got.Status != store.OrderStatusShipped || got.CompletedAt != nil || got.ReturnWindowEndsAt != nil || got.AutoCompleteAt == nil {
		t.Fatalf("order must stay SHIPPED with its deadline: %+v", got)
	}
}

func TestCompletionRollsBackWhenOutboxFails(t *testing.T) {
	pool := pgPool(t)
	runCompletionRollbackContract(t, func(t *testing.T, opts ...store.OrderRepoOption) store.OrderRepository {
		return store.NewPostgresOrderRepository(pool, newOutbox(t, pool), opts...)
	})
}

func TestOrderCompletion(t *testing.T) {
	pool := pgPool(t)
	f := func(t *testing.T, extra ...store.OrderRepoOption) rcFixture {
		orders := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), append(rcBaseOpts(), extra...)...)
		return rcFixture{
			repo: orders, placer: orders, sagas: store.NewPostgresSagaRepository(pool),
			rows: func(t *testing.T, id string) []*eventsv1.EventEnvelope {
				r, err := pool.Query(context.Background(), `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1 ORDER BY created_at`, id)
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
				return rcDecode(t, payloads)
			},
		}
	}
	runCompletionContract(t, f)
	runShipByPlacementContract(t, f)

	// Scenario: Order receipt: A changed config does not move an existing ship_by
	t.Run("TestShipByIsSnapshottedAtPayment_ConfigChange", func(t *testing.T) {
		ctx := context.Background()
		before := f(t)
		o := rcNewOrder(ctx, t, before.repo, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO)
		paid, err := before.repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid, fromPending, "")
		if err != nil {
			t.Fatal(err)
		}
		after := f(t, store.WithShipByAfter(time.Hour)) // restarted with a changed SHIP_BY_AFTER
		got, err := after.repo.GetOrder(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ShipBy == nil || got.ShipBy.Sub(*paid.ShipBy).Abs() > time.Millisecond {
			t.Fatalf("ship_by moved: %v -> %v", paid.ShipBy, got.ShipBy)
		}
	})
}

func runShipByPlacementContract(t *testing.T, fx rcFactory) {
	ctx := context.Background()
	place := func(t *testing.T, f rcFixture, method paymentv1.PaymentMethod, deadline time.Duration) (store.Order, *orderv1.OrderPlaced) {
		t.Helper()
		sagaID, res := newCheckout(ctx, t, f.sagas, "la")
		po := placedOrder(uid("ord"), "sa", method, deadline)
		po.ReservationIDs = res
		if _, err := f.placer.PlaceOrders(ctx, sagaID, []store.PlacedOrder{po}); err != nil {
			t.Fatal(err)
		}
		got, err := f.repo.GetOrder(ctx, po.Order.ID)
		if err != nil {
			t.Fatal(err)
		}
		var placed []*orderv1.OrderPlaced
		for _, e := range f.rows(t, got.ID) {
			if e.GetType() != OrderPlacedEventType {
				continue
			}
			var ev orderv1.OrderPlaced
			if err := proto.Unmarshal(e.GetPayload(), &ev); err != nil {
				t.Fatal(err)
			}
			placed = append(placed, &ev)
		}
		if len(placed) != 1 {
			t.Fatalf("want 1 OrderPlaced, got %d", len(placed))
		}
		return got, placed[0]
	}

	// Scenario: Order receipt: A cash-on-delivery order gets ship_by at placement
	t.Run("TestCodOrderGetsShipByAtPlacement", func(t *testing.T) {
		f := fx(t)
		got, ev := place(t, f, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, 0)
		rcNear(t, "ship_by", got.ShipBy, got.CreatedAt.Add(rcShipBy))
		if ev.GetShipBy() == nil || ev.GetShipBy().AsTime().Sub(got.ShipBy.UTC()).Abs() > time.Millisecond {
			t.Fatalf("OrderPlaced.ship_by %v, order %v", ev.GetShipBy(), got.ShipBy)
		}
	})

	// Scenario: Order receipt: An unpaid online order has no ship_by
	t.Run("TestUnpaidOnlineOrderHasNoShipBy", func(t *testing.T) {
		f := fx(t)
		got, ev := place(t, f, paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO, testPaymentDeadline)
		if got.ShipBy != nil || ev.GetShipBy() != nil {
			t.Fatalf("online PENDING order must have no ship_by: order=%v event=%v", got.ShipBy, ev.GetShipBy())
		}
	})
}
