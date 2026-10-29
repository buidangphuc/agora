package events_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

const (
	outboxTable         = "order_outbox_events"
	paymentConsumerName = "team-order.payment"
	disputeConsumerName = "team-order.dispute"
	paymentSettledType  = "platform.payment.v1.PaymentSettled"
)

var (
	pendingOnly = []store.OrderStatus{store.OrderStatusPending}
	cancelFrom  = []store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}
)

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

type rig struct {
	t       *testing.T
	pool    *pgxpool.Pool
	ob      *outbox.Store
	orders  *store.PostgresOrderRepository
	returns *store.PostgresReturnRepository
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Up(t.Context(), dsn, os.DirFS("../../migrations")); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	ob, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return &rig{
		t:    t,
		pool: pool,
		ob:   ob,
		orders: store.NewPostgresOrderRepository(pool, ob,
			store.WithPaidOutbox(events.BuildPaidOutboxRow),
			store.WithCancelledOutbox(events.BuildCancelledOutboxRow)),
		returns: store.NewPostgresReturnRepository(pool, ob,
			store.WithReturnOutbox(events.BuildReturnRefundedOutboxRow)),
	}
}

func (r *rig) count(query string, args ...any) int {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		r.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (r *rig) outboxRows(orderID string) int {
	r.t.Helper()
	return r.count(`SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID)
}

func (r *rig) outboxRowsOfType(orderID, eventType string) int {
	r.t.Helper()
	return r.count(`SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1 AND event_type = $2`, orderID, eventType)
}

func (r *rig) processedRows(consumerName, eventID string) int {
	r.t.Helper()
	return r.count(`SELECT count(*) FROM processed_events WHERE consumer = $1 AND event_id = $2`, consumerName, eventID)
}

func (r *rig) createOrder(o store.Order) store.Order {
	r.t.Helper()
	if o.BuyerID == "" {
		o.BuyerID = "buyer-1"
	}
	if o.SellerID == "" {
		o.SellerID = "seller-1"
	}
	if o.TotalAmount == 0 {
		o.TotalAmount = 10
	}
	if len(o.Items) == 0 {
		o.Items = []store.OrderItem{{ListingID: "lst_1", Quantity: 1, UnitPrice: o.TotalAmount}}
	}
	created, err := r.orders.CreateOrder(context.Background(), o)
	if err != nil {
		r.t.Fatalf("create order: %v", err)
	}
	return created
}

func (r *rig) order(id string) store.Order {
	r.t.Helper()
	o, err := r.orders.GetOrder(context.Background(), id)
	if err != nil {
		r.t.Fatalf("get order %s: %v", id, err)
	}
	return o
}

func (r *rig) pay(id string) {
	r.t.Helper()
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), id, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		r.t.Fatalf("pay %s: %v", id, err)
	}
}

func sampleOrder() store.Order {
	return store.Order{
		ID:          "order-123",
		BuyerID:     "buyer-1",
		SellerID:    "seller-1",
		Status:      store.OrderStatusPending,
		TotalAmount: 250000,
		Currency:    "VND",
		Items: []store.OrderItem{
			{ID: "item-1", ListingID: "listing-1", VariantID: "var-1", Quantity: 2, UnitPrice: 100000},
			{ID: "item-2", ListingID: "listing-2", Quantity: 1, UnitPrice: 50000},
		},
	}
}

func settledEnvelope(t *testing.T, eventID string, settled *paymentv1.PaymentSettled) []byte {
	t.Helper()
	payload, err := proto.Marshal(settled)
	if err != nil {
		t.Fatalf("marshal settled: %v", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: paymentSettledType, Payload: payload})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return value
}

func paidSettlement(orderID string) *paymentv1.PaymentSettled {
	return &paymentv1.PaymentSettled{PaymentId: "pay-" + orderID, OrderId: orderID, Status: paymentv1.PaymentStatus_PAYMENT_STATUS_PAID}
}

func paymentRecord(t *testing.T, eventID string, settled *paymentv1.PaymentSettled) consume.Record {
	t.Helper()
	return consume.Record{Topic: "payment.events", Key: []byte(settled.GetOrderId()), Value: settledEnvelope(t, eventID, settled)}
}
