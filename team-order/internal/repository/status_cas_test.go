package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
)

func paidRow(o repository.Order) (repository.OutboxRow, error) {
	return repository.OutboxRow{EventID: "paid-" + o.ID, AggregateType: "Order", AggregateID: o.ID,
		EventType: "platform.order.v1.OrderPaidEvent", Payload: []byte("env")}, nil
}

var (
	fromPending     = []repository.OrderStatus{repository.OrderStatusPending}
	fromPendingPaid = []repository.OrderStatus{repository.OrderStatusPending, repository.OrderStatusPaid}
)

func runStatusCASContract(t *testing.T, repo repository.OrderRepository, outboxRows func(orderID string) int) {
	ctx := context.Background()
	o, err := repo.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10,
		Items: []repository.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: 10}}})
	if err != nil {
		t.Fatal(err)
	}

	paid, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, "")
	if err != nil || paid.Status != repository.OrderStatusPaid || paid.PaidAt == nil || len(paid.Items) != 1 {
		t.Fatalf("pay: %v %+v", err, paid)
	}
	if n := outboxRows(o.ID); n != 1 {
		t.Fatalf("want 1 OrderPaid row, got %d", n)
	}
	if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, ""); !errors.Is(err, repository.ErrStatusConflict) {
		t.Fatalf("second pay: want ErrStatusConflict, got %v", err)
	}
	if n := outboxRows(o.ID); n != 1 {
		t.Fatalf("a conflicting write must not emit: %d rows", n)
	}
	if _, err := repo.UpdateOrderStatusFrom(ctx, "missing-"+o.ID, repository.OrderStatusPaid, fromPending, ""); !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("want ErrOrderNotFound, got %v", err)
	}
	shipped, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{repository.OrderStatusPaid}, "TRK-"+o.ID)
	if err != nil || shipped.TrackingNumber != "TRK-"+o.ID || shipped.PaidAt == nil {
		t.Fatalf("ship: %v %+v", err, shipped)
	}
	if got, _ := repo.GetOrder(ctx, o.ID); got.PaidAt == nil || got.Status != repository.OrderStatusShipped {
		t.Fatalf("paid_at must persist: %+v", got)
	}

	// Two racing cancels: exactly one wins.
	o2, _ := repo.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10})
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, conflicts := 0, 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.UpdateOrderStatusFrom(ctx, o2.ID, repository.OrderStatusCancelled, fromPendingPaid, "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, repository.ErrStatusConflict):
				conflicts++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins != 1 || conflicts != 3 {
		t.Fatalf("wins=%d conflicts=%d, want 1/3", wins, conflicts)
	}
}

func TestUpdateOrderStatusFrom_InMemory(t *testing.T) {
	outbox := repository.NewInMemoryOutboxRepository()
	repo := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(paidRow), repository.WithInMemoryOutbox(outbox))
	runStatusCASContract(t, repo, func(id string) int {
		n := 0
		for _, r := range outbox.EnqueuedRows() {
			if r.AggregateID == id {
				n++
			}
		}
		return n
	})
}

func TestUpdateOrderStatusFrom_Postgres(t *testing.T) {
	pool := pgPool(t)
	repo := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(paidRow))
	runStatusCASContract(t, repo, func(id string) int {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	})
	// The database backstop rejects an out-of-range status.
	if _, err := pool.Exec(context.Background(), `INSERT INTO orders (id, buyer_id, seller_id, status, total_amount) VALUES ($1, 'b', 's', 9, 1)`, uid("bad")); err == nil {
		t.Fatal("orders_status_check must reject status 9")
	}
}
