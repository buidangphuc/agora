package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

// returnRowsFn counts the outbox rows written for one return.
type returnRowsFn func(t *testing.T, orderID, returnID string) int

// fakeReturnOutbox is a minimal ReturnOutboxBuilder: event id = return id, so a
// test can count rows per return without the events package.
func fakeReturnOutbox(ret store.OrderReturn, currency string) (outbox.Row, error) {
	return outbox.Row{
		EventID:       "rr-" + ret.ID,
		AggregateType: "Order",
		AggregateID:   ret.OrderID,
		EventType:     "test.ReturnRefunded",
		Payload:       []byte(ret.ID + "|" + currency),
	}, nil
}

type returnFixture struct {
	orders  store.OrderRepository
	returns store.ReturnRepository
	rows    returnRowsFn
}

func newReturnOrder(t *testing.T, f returnFixture, total int64) store.Order {
	t.Helper()
	o, err := f.orders.CreateOrder(context.Background(), store.Order{BuyerID: uid("b"), SellerID: uid("s"),
		TotalAmount: total, Currency: "VND", Items: []store.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: total}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func newReturn(t *testing.T, f returnFixture, o store.Order, amount int64) store.OrderReturn {
	t.Helper()
	r, err := f.returns.CreateReturnCapped(context.Background(), store.OrderReturn{OrderID: o.ID, BuyerID: o.BuyerID,
		SellerID: o.SellerID, Reason: "broken", RefundAmount: amount}, o.TotalAmount)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func runReturnRepoContract(t *testing.T, f returnFixture) {
	ctx := context.Background()
	P, A, J, R := store.ReturnStatusPending, store.ReturnStatusApproved, store.ReturnStatusRejected, store.ReturnStatusRefunded

	t.Run("won approved to refunded writes one outbox row", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		r := newReturn(t, f, o, 200000)
		if _, err := f.returns.TransitionReturn(ctx, r.ID, P, A); err != nil {
			t.Fatal(err)
		}
		if n := f.rows(t, o.ID, r.ID); n != 0 {
			t.Fatalf("approve must write no row, got %d", n)
		}
		got, err := f.returns.TransitionReturn(ctx, r.ID, A, R)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != R || got.RefundAmount != 200000 {
			t.Fatalf("returned row: %+v", got)
		}
		if n := f.rows(t, o.ID, r.ID); n != 1 {
			t.Fatalf("want 1 row, got %d", n)
		}
		// A replay of the same transition loses the CAS and writes nothing more.
		if _, err := f.returns.TransitionReturn(ctx, r.ID, A, R); !errors.Is(err, store.ErrReturnStatusConflict) {
			t.Fatalf("want ErrReturnStatusConflict, got %v", err)
		}
		if n := f.rows(t, o.ID, r.ID); n != 1 {
			t.Fatalf("lost CAS must not write: %d rows", n)
		}
	})

	t.Run("lost CAS and other transitions write none", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		r := newReturn(t, f, o, 100000)
		// Pending -> Refunded via a wrong `from` loses the CAS.
		if _, err := f.returns.TransitionReturn(ctx, r.ID, A, R); !errors.Is(err, store.ErrReturnStatusConflict) {
			t.Fatalf("want conflict, got %v", err)
		}
		if got, _ := f.returns.GetReturn(ctx, r.ID); got.Status != P {
			t.Fatalf("status must stay PENDING: %v", got.Status)
		}
		if _, err := f.returns.TransitionReturn(ctx, r.ID, P, A); err != nil {
			t.Fatal(err)
		}
		if _, err := f.returns.TransitionReturn(ctx, r.ID, A, J); err != nil {
			t.Fatal(err)
		}
		if n := f.rows(t, o.ID, r.ID); n != 0 {
			t.Fatalf("approve/reject must write none, got %d", n)
		}
		if _, err := f.returns.TransitionReturn(ctx, "missing-"+r.ID, P, A); !errors.Is(err, store.ErrReturnNotFound) {
			t.Fatalf("want not found, got %v", err)
		}
	})

	t.Run("eight concurrent refunds of one return give one winner and one row", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		r := newReturn(t, f, o, 200000)
		if _, err := f.returns.TransitionReturn(ctx, r.ID, P, A); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		wins, conflicts := 0, 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.returns.TransitionReturn(ctx, r.ID, A, R)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					wins++
				case errors.Is(err, store.ErrReturnStatusConflict):
					conflicts++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		if wins != 1 || conflicts != 7 {
			t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
		}
		if n := f.rows(t, o.ID, r.ID); n != 1 {
			t.Fatalf("want 1 row, got %d", n)
		}
	})

	t.Run("eight concurrent return requests of 100000 on 500000 give five inserts", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, over := 0, 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.returns.CreateReturnCapped(ctx, store.OrderReturn{OrderID: o.ID, BuyerID: o.BuyerID,
					SellerID: o.SellerID, Reason: "r", RefundAmount: 100000}, o.TotalAmount)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, store.ErrReturnExceedsRemainder):
					over++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		if ok != 5 || over != 3 {
			t.Fatalf("ok=%d refused=%d", ok, over)
		}
		list, err := f.returns.ListReturnsByOrder(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		var sum int64
		for _, r := range list {
			sum += r.RefundAmount
		}
		if len(list) != 5 || sum != 500000 {
			t.Fatalf("listed %d returns totalling %d", len(list), sum)
		}
	})

	t.Run("cap rules and a rejected return frees its amount", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		first := newReturn(t, f, o, 300000)
		if _, err := f.returns.CreateReturnCapped(ctx, store.OrderReturn{OrderID: o.ID, BuyerID: o.BuyerID, SellerID: o.SellerID,
			Reason: "r", RefundAmount: 300000}, o.TotalAmount); !errors.Is(err, store.ErrReturnExceedsRemainder) {
			t.Fatalf("want ErrReturnExceedsRemainder, got %v", err)
		}
		// No amount defaults to the remainder.
		rest := newReturn(t, f, o, 0)
		if rest.RefundAmount != 200000 {
			t.Fatalf("default amount: %d", rest.RefundAmount)
		}
		if _, err := f.returns.CreateReturnCapped(ctx, store.OrderReturn{OrderID: o.ID, BuyerID: o.BuyerID, SellerID: o.SellerID,
			Reason: "r"}, o.TotalAmount); !errors.Is(err, store.ErrNoReturnableRemainder) {
			t.Fatalf("want ErrNoReturnableRemainder, got %v", err)
		}
		if _, err := f.returns.CreateReturnCapped(ctx, store.OrderReturn{OrderID: o.ID, BuyerID: o.BuyerID, SellerID: o.SellerID,
			Reason: "r", RefundAmount: 1}, o.TotalAmount); !errors.Is(err, store.ErrReturnExceedsRemainder) {
			t.Fatalf("a positive amount with nothing left: want ErrReturnExceedsRemainder, got %v", err)
		}
		if _, err := f.returns.TransitionReturn(ctx, first.ID, P, J); err != nil {
			t.Fatal(err)
		}
		again := newReturn(t, f, o, 300000)
		if again.Status != P {
			t.Fatalf("new return status %v", again.Status)
		}
	})

	t.Run("list by order is newest first", func(t *testing.T) {
		o := newReturnOrder(t, f, 500000)
		a := newReturn(t, f, o, 100000)
		time.Sleep(2 * time.Millisecond)
		b := newReturn(t, f, o, 200000)
		list, err := f.returns.ListReturnsByOrder(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
			t.Fatalf("order: %+v", list)
		}
		if empty, err := f.returns.ListReturnsByOrder(ctx, uid("none")); err != nil || len(empty) != 0 {
			t.Fatalf("unknown order: %v %v", empty, err)
		}
	})
}

func TestReturnRepoContract_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	orders := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
	returns := store.NewPostgresReturnRepository(pool, newOutbox(t, pool), store.WithReturnOutbox(fakeReturnOutbox))
	rows := func(t *testing.T, orderID, returnID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1 AND event_id = $2`,
			orderID, "rr-"+returnID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	runReturnRepoContract(t, returnFixture{orders: orders, returns: returns, rows: rows})

	// Rollback: a failing builder undoes the transition, and nothing is written.
	failing := store.NewPostgresReturnRepository(pool, newOutbox(t, pool), store.WithReturnOutbox(
		func(store.OrderReturn, string) (outbox.Row, error) {
			return outbox.Row{}, errors.New("boom")
		}))
	f := returnFixture{orders: orders, returns: failing, rows: rows}
	o := newReturnOrder(t, f, 500000)
	r := newReturn(t, f, o, 200000)
	if _, err := failing.TransitionReturn(ctx, r.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
		t.Fatal(err)
	}
	if _, err := failing.TransitionReturn(ctx, r.ID, store.ReturnStatusApproved, store.ReturnStatusRefunded); err == nil {
		t.Fatal("expected error from failing return outbox builder")
	}
	if got, _ := failing.GetReturn(ctx, r.ID); got.Status != store.ReturnStatusApproved {
		t.Fatalf("transition must roll back, status %v", got.Status)
	}
	if n := rows(t, o.ID, r.ID); n != 0 {
		t.Fatalf("want 0 rows after rollback, got %d", n)
	}
	// The currency reaches the builder from the order.
	var payload []byte
	ok := newReturn(t, returnFixture{orders: orders, returns: returns}, o, 100000)
	if _, err := returns.TransitionReturn(ctx, ok.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
		t.Fatal(err)
	}
	if _, err := returns.TransitionReturn(ctx, ok.ID, store.ReturnStatusApproved, store.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT payload FROM order_outbox_events WHERE event_id = $1`, "rr-"+ok.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != ok.ID+"|VND" {
		t.Fatalf("payload %q", payload)
	}
}

// With the real builder, the stored row carries the stable id, the type, the
// order key, the stored amount and the order currency.
func TestReturnRefundedOutbox_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	orders := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
	returns := store.NewPostgresReturnRepository(pool, newOutbox(t, pool), store.WithReturnOutbox(BuildReturnRefundedOutboxRow))
	f := returnFixture{orders: orders, returns: returns}
	o := newReturnOrder(t, f, 500000)
	r := newReturn(t, f, o, 200000)
	if _, err := returns.TransitionReturn(ctx, r.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
		t.Fatal(err)
	}
	if _, err := returns.TransitionReturn(ctx, r.ID, store.ReturnStatusApproved, store.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}
	var eventID, eventType string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT event_id, event_type, payload FROM order_outbox_events WHERE aggregate_id = $1`, o.ID).
		Scan(&eventID, &eventType, &payload); err != nil {
		t.Fatal(err)
	}
	if eventID != ReturnRefundedEventID(r.ID) || eventType != ReturnRefundedEventType {
		t.Fatalf("row id/type: %s %s", eventID, eventType)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		t.Fatal(err)
	}
	var ev orderv1.ReturnRefunded
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.GetReturnId() != r.ID || ev.GetOrderId() != o.ID || ev.GetBuyerId() != o.BuyerID || ev.GetSellerId() != o.SellerID ||
		ev.GetRefundAmount() != 200000 || ev.GetCurrency() != "VND" || ev.GetRefundedAt() == nil {
		t.Fatalf("fact: %+v", &ev)
	}
}
