package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-order/internal/repository"
)

// Postgres check that the OrderShipped outbox row commits and rolls back with the
// shipment. Skips without TEST_DATABASE_URL/DATABASE_URL.
func TestCreateShipment_OutboxSameTransaction_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL/DATABASE_URL set; skipping Postgres shipment outbox test")
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

	count := func(orderID string) (outbox, shipments int) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID).Scan(&outbox); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM shipments WHERE order_id = $1`, orderID).Scan(&shipments); err != nil {
			t.Fatalf("count shipments: %v", err)
		}
		return outbox, shipments
	}

	ok := repository.NewPostgresShipmentRepository(pool, repository.WithShipmentOutbox(
		func(s repository.Shipment) (repository.OutboxRow, error) {
			return repository.OutboxRow{EventID: "evt-ship-" + s.ID, AggregateType: "Order", AggregateID: s.OrderID,
				EventType: "platform.order.v1.OrderShipped", Payload: []byte("env")}, nil
		}))
	if _, err := ok.CreateShipment(ctx, repository.Shipment{OrderID: "pg-ship-ok", Carrier: "SPX", TrackingCode: "PG-OK"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if o, s := count("pg-ship-ok"); o != 1 || s != 1 {
		t.Fatalf("commit path: outbox=%d shipments=%d, want 1/1", o, s)
	}

	bad := repository.NewPostgresShipmentRepository(pool, repository.WithShipmentOutbox(
		func(repository.Shipment) (repository.OutboxRow, error) {
			return repository.OutboxRow{}, errors.New("boom")
		}))
	if _, err := bad.CreateShipment(ctx, repository.Shipment{OrderID: "pg-ship-bad", Carrier: "SPX", TrackingCode: "PG-BAD"}); err == nil {
		t.Fatal("expected error from failing outbox builder")
	}
	if o, s := count("pg-ship-bad"); o != 0 || s != 0 {
		t.Fatalf("rollback path: outbox=%d shipments=%d, want 0/0", o, s)
	}
}
