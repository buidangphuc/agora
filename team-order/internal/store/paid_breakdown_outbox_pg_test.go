package store_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

var pendingOnly = []store.OrderStatus{store.OrderStatusPending}

func newBreakdownOrder(t *testing.T, repo store.OrderRepository) store.Order {
	t.Helper()
	o, err := repo.CreateOrder(t.Context(), store.Order{
		BuyerID: "b", SellerID: "s", Currency: "VND",
		ItemsSubtotal: 300000, ShippingFee: 30000, DiscountAmount: 50000, PlatformDiscountAmount: 30000, TotalAmount: 280000,
		Items: []store.OrderItem{{ListingID: "l1", Quantity: 1, UnitPrice: 300000}},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return o
}

func storedPaidPayloads(t *testing.T, pool *pgxpool.Pool, orderID string) [][]byte {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1 ORDER BY seq`, orderID)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var p []byte
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func decodePaid(t *testing.T, value []byte) *orderv1.OrderPaidEvent {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var paid orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.Payload, &paid); err != nil {
		t.Fatalf("unmarshal OrderPaidEvent: %v", err)
	}
	return &paid
}

func requireBreakdown(t *testing.T, p *orderv1.OrderPaidEvent, subtotal, shipping, shop, platform int64) {
	t.Helper()
	if p.ItemsSubtotal == nil || p.ShippingFee == nil || p.ShopDiscountAmount == nil || p.PlatformDiscountAmount == nil {
		t.Fatalf("breakdown fields absent: %+v", p)
	}
	if *p.ItemsSubtotal != subtotal || *p.ShippingFee != shipping || *p.ShopDiscountAmount != shop || *p.PlatformDiscountAmount != platform {
		t.Fatalf("breakdown = %d/%d/%d/%d, want %d/%d/%d/%d", *p.ItemsSubtotal, *p.ShippingFee, *p.ShopDiscountAmount, *p.PlatformDiscountAmount, subtotal, shipping, shop, platform)
	}
}

// Scenario: Breakdown is committed in the PAID transaction
func TestPaidTransitionOutboxCarriesBreakdown(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(events.BuildPaidOutboxRow))
	o := newBreakdownOrder(t, repo)

	if _, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		t.Fatalf("pay: %v", err)
	}
	payloads := storedPaidPayloads(t, pool, o.ID)
	if len(payloads) != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", len(payloads))
	}
	paid := decodePaid(t, payloads[0])
	requireBreakdown(t, paid, 300000, 30000, 20000, 30000)
	if paid.TotalAmount != 280000 || paid.OrderId != o.ID {
		t.Fatalf("legacy fields changed: %+v", paid)
	}
}

// Scenario: A failing enqueue rolls the PAID transition back
func TestPaidTransitionRollsBackOnEnqueueFailure(t *testing.T) {
	pool := pgPool(t)
	failing := func(o store.Order) (outbox.Row, error) {
		row, err := events.BuildPaidOutboxRow(o)
		row.EventID = ""
		return row, err
	}
	bad := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(failing))
	o := newBreakdownOrder(t, bad)

	if _, err := bad.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pendingOnly, ""); err == nil {
		t.Fatal("pay must fail when the outbox insert is rejected")
	}
	got, err := bad.GetOrder(t.Context(), o.ID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if got.Status != store.OrderStatusPending || got.PaidAt != nil {
		t.Fatalf("order = %+v, want PENDING and unpaid", got)
	}
	if n := outboxRows(t, pool, o.ID); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}

	good := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(events.BuildPaidOutboxRow))
	if _, err := good.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		t.Fatalf("retry pay: %v", err)
	}
	payloads := storedPaidPayloads(t, pool, o.ID)
	if len(payloads) != 1 {
		t.Fatalf("outbox rows after retry = %d, want 1", len(payloads))
	}
	requireBreakdown(t, decodePaid(t, payloads[0]), 300000, 30000, 20000, 30000)
}

// Scenario: Outbox rows enqueued before deploy are not rewritten
func TestPendingLegacyPaidRowRelayedUnchanged(t *testing.T) {
	pool := pgPool(t)
	legacyOrderID := uid("legacy")
	paidAt := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	legacyPayload, err := proto.Marshal(&orderv1.OrderPaidEvent{OrderId: legacyOrderID, BuyerId: "b", SellerId: "s", TotalAmount: 280000, Currency: "VND"})
	if err != nil {
		t.Fatalf("marshal legacy event: %v", err)
	}
	legacyValue, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: events.OrderPaidEventID(legacyOrderID), Type: events.OrderPaidEventType, Payload: legacyPayload})
	if err != nil {
		t.Fatalf("marshal legacy envelope: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO order_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at)
		VALUES ($1, 'Order', $2, $3, $4, '', 'pending', 0, $5, $5)`,
		events.OrderPaidEventID(legacyOrderID), legacyOrderID, events.OrderPaidEventType, legacyValue, paidAt); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(events.BuildPaidOutboxRow))
	fresh := newBreakdownOrder(t, repo)
	if _, err := repo.UpdateOrderStatusFrom(t.Context(), fresh.ID, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		t.Fatalf("pay fresh order: %v", err)
	}

	rec := testkit.NewRecorder()
	drain(t, newRelayer(t, pool, rec))
	var sawLegacy, sawFresh bool
	for _, r := range rec.Records() {
		switch string(r.Key) {
		case legacyOrderID:
			sawLegacy = true
			if !bytes.Equal(r.Value, legacyValue) {
				t.Fatal("legacy payload was rewritten")
			}
			p := decodePaid(t, r.Value)
			if p.ItemsSubtotal != nil || p.ShippingFee != nil || p.ShopDiscountAmount != nil || p.PlatformDiscountAmount != nil {
				t.Fatalf("legacy payload gained breakdown fields: %+v", p)
			}
		case fresh.ID:
			sawFresh = true
			requireBreakdown(t, decodePaid(t, r.Value), 300000, 30000, 20000, 30000)
		}
	}
	if !sawLegacy || !sawFresh {
		t.Fatalf("published legacy=%v fresh=%v, want both", sawLegacy, sawFresh)
	}
}

// Scenario: Re-emitted PAID fact collapses to one row
func TestPaidReplayKeepsSingleBreakdownRow(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(events.BuildPaidOutboxRow))
	o := newBreakdownOrder(t, repo)
	if _, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		t.Fatalf("pay: %v", err)
	}
	first := storedPaidPayloads(t, pool, o.ID)
	if len(first) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(first))
	}
	requireBreakdown(t, decodePaid(t, first[0]), 300000, 30000, 20000, 30000)

	if _, err := pool.Exec(t.Context(), `UPDATE orders SET status = 1, paid_at = NULL, discount_amount = 0, platform_discount_amount = 0, total_amount = 330000 WHERE id = $1`, o.ID); err != nil {
		t.Fatalf("rewind order: %v", err)
	}
	if _, err := repo.UpdateOrderStatusFrom(t.Context(), o.ID, store.OrderStatusPaid, pendingOnly, ""); err != nil {
		t.Fatalf("replay pay: %v", err)
	}
	after := storedPaidPayloads(t, pool, o.ID)
	if len(after) != 1 {
		t.Fatalf("outbox rows after replay = %d, want 1", len(after))
	}
	if !bytes.Equal(after[0], first[0]) {
		t.Fatal("replay rewrote the first payload")
	}
}
