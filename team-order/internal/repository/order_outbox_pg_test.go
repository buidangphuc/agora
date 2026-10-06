package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-order/internal/repository"
)

// Postgres check that the order.events outbox row commits and rolls back with the
// PAID status change. Runs only when TEST_DATABASE_URL (or DATABASE_URL) points at
// a reachable Postgres; otherwise it skips so the unit suite needs no database.
func TestPaidTransition_OutboxSameTransaction_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL/DATABASE_URL set; skipping Postgres outbox test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	applyMigrations(t, ctx, pool)

	build := func(o repository.Order) (repository.OutboxRow, error) {
		return repository.OutboxRow{
			EventID: "evt-" + o.ID, AggregateType: "Order", AggregateID: o.ID,
			EventType: "platform.order.v1.OrderPaidEvent", Payload: []byte("env"),
		}, nil
	}
	count := func(orderID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID).Scan(&n); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		return n
	}

	// Commit path: status and outbox row land together.
	repo := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(build))
	o, err := repo.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10,
		Items: []repository.OrderItem{{ListingID: "l1", Quantity: 1, UnitPrice: 10}}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got, err := repo.UpdateOrderStatus(ctx, o.ID, repository.OrderStatusPaid, ""); err != nil || got.Status != repository.OrderStatusPaid {
		t.Fatalf("pay: %v %+v", err, got)
	}
	if n := count(o.ID); n != 1 {
		t.Fatalf("want 1 outbox row after PAID, got %d", n)
	}
	// Re-applying PAID does not emit again.
	if _, err := repo.UpdateOrderStatus(ctx, o.ID, repository.OrderStatusPaid, ""); err != nil {
		t.Fatalf("re-pay: %v", err)
	}
	if n := count(o.ID); n != 1 {
		t.Fatalf("want still 1 outbox row, got %d", n)
	}

	// Rollback path: a failing outbox write undoes the status change.
	failing := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(
		func(repository.Order) (repository.OutboxRow, error) {
			return repository.OutboxRow{}, errors.New("boom")
		}))
	o2, err := failing.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10})
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	if _, err := failing.UpdateOrderStatus(ctx, o2.ID, repository.OrderStatusPaid, ""); err == nil {
		t.Fatal("expected error from failing outbox builder")
	}
	got, err := failing.GetOrder(ctx, o2.ID)
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if got.Status != repository.OrderStatusPending {
		t.Fatalf("status must roll back to PENDING, got %v", got.Status)
	}
	if n := count(o2.ID); n != 0 {
		t.Fatalf("want 0 outbox rows after rollback, got %d", n)
	}
}
