package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
)

// cancelRows returns the decoded OrderCancelled facts written for an order.
type cancelRowsFn func(t *testing.T, orderID string) []*orderv1.OrderCancelled

func decodeCancelPayloads(t *testing.T, payloads [][]byte) []*orderv1.OrderCancelled {
	t.Helper()
	var out []*orderv1.OrderCancelled
	for _, p := range payloads {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(p, &env); err != nil {
			t.Fatal(err)
		}
		if env.GetType() != events.OrderCancelledEventType {
			continue
		}
		var ev orderv1.OrderCancelled
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func newCancelOrder(t *testing.T, repo repository.OrderRepository) repository.Order {
	t.Helper()
	o, err := repo.CreateOrder(context.Background(), repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10,
		Currency: "VND", Items: []repository.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// runCancelledOutboxContract: one OrderCancelled row per won cancel, from Pending
// and from Paid with the right previous_status; none for a lost claim and none
// for any other transition.
func runCancelledOutboxContract(t *testing.T, repo repository.OrderRepository, rows cancelRowsFn) {
	ctx := context.Background()

	t.Run("won cancel from pending", func(t *testing.T) {
		o := newCancelOrder(t, repo)
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, ""); err != nil {
			t.Fatal(err)
		}
		got := rows(t, o.ID)
		if len(got) != 1 {
			t.Fatalf("want 1 OrderCancelled row, got %d", len(got))
		}
		ev := got[0]
		if ev.GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PENDING || ev.GetOrderId() != o.ID ||
			ev.GetBuyerId() != "b" || ev.GetSellerId() != "s" || ev.GetTotalAmount() != 10 || ev.GetCancelledAt() == nil {
			t.Fatalf("fact: %+v", ev)
		}
	})

	t.Run("won cancel from paid", func(t *testing.T) {
		o := newCancelOrder(t, repo)
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, ""); err != nil {
			t.Fatal(err)
		}
		if n := len(rows(t, o.ID)); n != 0 {
			t.Fatalf("the move to Paid must not write an OrderCancelled row: %d", n)
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, ""); err != nil {
			t.Fatal(err)
		}
		got := rows(t, o.ID)
		if len(got) != 1 || got[0].GetPreviousStatus() != orderv1.OrderStatus_ORDER_STATUS_PAID {
			t.Fatalf("want 1 OrderCancelled row from PAID, got %+v", got)
		}
	})

	t.Run("lost claims write nothing", func(t *testing.T) {
		o := newCancelOrder(t, repo)
		var wg sync.WaitGroup
		var mu sync.Mutex
		wins := 0
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, "")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					wins++
				case errors.Is(err, repository.ErrStatusConflict):
				default:
					t.Errorf("unexpected: %v", err)
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("wins=%d, want 1", wins)
		}
		if n := len(rows(t, o.ID)); n != 1 {
			t.Fatalf("want exactly 1 OrderCancelled row for 6 racing cancels, got %d", n)
		}
	})

	t.Run("other transitions write no cancel fact", func(t *testing.T) {
		o := newCancelOrder(t, repo)
		paid := []repository.OrderStatus{repository.OrderStatusPaid}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, paid, "TRK"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, ""); !errors.Is(err, repository.ErrStatusConflict) {
			t.Fatalf("a shipped order cannot be cancelled: %v", err)
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCompleted, []repository.OrderStatus{repository.OrderStatusShipped}, ""); err != nil {
			t.Fatal(err)
		}
		if n := len(rows(t, o.ID)); n != 0 {
			t.Fatalf("want 0 OrderCancelled rows, got %d", n)
		}
	})
}

func TestCancelledOutbox_InMemory(t *testing.T) {
	outbox := repository.NewInMemoryOutboxRepository()
	repo := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow), repository.WithInMemoryOutbox(outbox))
	runCancelledOutboxContract(t, repo, func(t *testing.T, id string) []*orderv1.OrderCancelled {
		var payloads [][]byte
		for _, r := range outbox.EnqueuedRows() {
			if r.AggregateID == id {
				payloads = append(payloads, r.Payload)
			}
		}
		return decodeCancelPayloads(t, payloads)
	})
}

func TestCancelledOutbox_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	repo := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow))
	rows := func(t *testing.T, id string) []*orderv1.OrderCancelled {
		r, err := pool.Query(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, id)
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
		return decodeCancelPayloads(t, payloads)
	}
	runCancelledOutboxContract(t, repo, rows)

	// The stored row carries the stable id and the order key.
	o := newCancelOrder(t, repo)
	if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, ""); err != nil {
		t.Fatal(err)
	}
	var eventID, eventType string
	if err := pool.QueryRow(ctx, `SELECT event_id, event_type FROM order_outbox_events WHERE aggregate_id = $1`, o.ID).
		Scan(&eventID, &eventType); err != nil {
		t.Fatal(err)
	}
	if eventID != events.OrderCancelledEventID(o.ID) || eventType != events.OrderCancelledEventType {
		t.Fatalf("row id/type: %s %s", eventID, eventType)
	}

	// Rollback: a failing builder undoes the cancel, and nothing is written.
	failing := repository.NewPostgresOrderRepository(pool, repository.WithCancelledOutbox(
		func(repository.Order) (repository.OutboxRow, error) {
			return repository.OutboxRow{}, errors.New("boom")
		}))
	o2 := newCancelOrder(t, failing)
	if _, err := failing.UpdateOrderStatusFrom(ctx, o2.ID, repository.OrderStatusCancelled, fromPendingPaid, ""); err == nil {
		t.Fatal("expected error from failing cancel outbox builder")
	}
	got, err := failing.GetOrder(ctx, o2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != repository.OrderStatusPending {
		t.Fatalf("cancel must roll back, status %v", got.Status)
	}
	if n := len(rows(t, o2.ID)); n != 0 {
		t.Fatalf("want 0 rows after rollback, got %d", n)
	}
}
