package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-order/internal/store"
)

// Postgres check that the OrderShipped outbox row commits and rolls back with the
// shipment. Skips without TEST_DATABASE_URL/DATABASE_URL.
func TestCreateShipment_OutboxSameTransaction_Postgres(t *testing.T) {
	ctx := context.Background()
	pool := pgPool(t)

	count := func(orderID string) (outbox, shipments int) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID).Scan(&outbox); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM shipments WHERE order_id = $1`, orderID).Scan(&shipments); err != nil {
			t.Fatalf("count shipments: %v", err)
		}
		return outbox, shipments
	}

	// shipments.order_id references orders: create the orders first, with ids and
	// tracking codes unique per run so the test can re-run against one database.
	orders := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
	okOrder, err := orders.CreateOrder(ctx, store.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	badOrder, err := orders.CreateOrder(ctx, store.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	ok := store.NewPostgresShipmentRepository(pool, newOutbox(t, pool), store.WithShipmentOutbox(
		func(s store.Shipment) (outbox.Row, error) {
			return outbox.Row{EventID: "evt-ship-" + s.ID, AggregateType: "Order", AggregateID: s.OrderID,
				EventType: "platform.order.v1.OrderShipped", Payload: []byte("env")}, nil
		}))
	if _, err := ok.CreateShipment(ctx, store.Shipment{OrderID: okOrder.ID, Carrier: "SPX", TrackingCode: "PG-OK-" + okOrder.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if o, s := count(okOrder.ID); o != 1 || s != 1 {
		t.Fatalf("commit path: outbox=%d shipments=%d, want 1/1", o, s)
	}

	bad := store.NewPostgresShipmentRepository(pool, newOutbox(t, pool), store.WithShipmentOutbox(
		func(store.Shipment) (outbox.Row, error) {
			return outbox.Row{}, errors.New("boom")
		}))
	if _, err := bad.CreateShipment(ctx, store.Shipment{OrderID: badOrder.ID, Carrier: "SPX", TrackingCode: "PG-BAD-" + badOrder.ID}); err == nil {
		t.Fatal("expected error from failing outbox builder")
	}
	if o, s := count(badOrder.ID); o != 0 || s != 0 {
		t.Fatalf("rollback path: outbox=%d shipments=%d, want 0/0", o, s)
	}
}
