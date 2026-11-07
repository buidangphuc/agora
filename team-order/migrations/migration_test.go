package migrations_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/migrations"
)

const (
	listingSalesUp   = "0023_listing_sales.up.sql"
	salesEventType   = "platform.order.v1.ListingSalesChanged"
	stPending        = 1
	stPaid           = 2
	stShipped        = 3
	stCompleted      = 4
	stCancelled      = 5
	mCOD             = 1
	mMomo            = 2
	returnApproved   = 2
	returnRefunded   = 4
	returnPendingRet = 1
)

type mdb struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	seq  int
}

func newMigDB(t *testing.T) *mdb {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= "0023" {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		before[e.Name()] = &fstest.MapFile{Data: b}
	}
	if err := migrate.Up(t.Context(), dsn, before); err != nil {
		t.Fatalf("migrate to 0022: %v", err)
	}
	return &mdb{t: t, ctx: context.Background(), pool: pool}
}

func (m *mdb) exec(sql string, args ...any) {
	m.t.Helper()
	if _, err := m.pool.Exec(m.ctx, sql, args...); err != nil {
		m.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (m *mdb) order(id string, status, method int, paid bool) {
	m.t.Helper()
	var paidAt *time.Time
	if paid {
		ts := time.Now().Add(-48 * time.Hour)
		paidAt = &ts
	}
	m.exec(`INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, currency, payment_method, paid_at, created_at, updated_at)
	        VALUES ($1, 'buyer', 'seller', $2, 1000, 'VND', $3, $4, now() - interval '3 days', now() - interval '2 days')`,
		id, status, method, paidAt)
}

func (m *mdb) line(orderID, listing string, qty int) {
	m.t.Helper()
	m.seq++
	m.exec(`INSERT INTO order_items (id, order_id, listing_id, variant_id, title, quantity, unit_price)
	        VALUES ($1, $2, $3, $4, 't', $5, 100)`,
		fmt.Sprintf("%s-i%d", orderID, m.seq), orderID, listing, fmt.Sprintf("v%d", m.seq), qty)
}

func (m *mdb) ret(id, orderID string, status int) {
	m.t.Helper()
	m.exec(`INSERT INTO order_returns (id, order_id, buyer_id, seller_id, reason, refund_amount, status)
	        VALUES ($1, $2, 'buyer', 'seller', 'damaged', 100, $3)`, id, orderID, status)
}

func readUp(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(listingSalesUp)
	if err != nil {
		t.Fatalf("read %s: %v", listingSalesUp, err)
	}
	return string(b)
}

func (m *mdb) runUp() time.Time {
	m.t.Helper()
	body := readUp(m.t)
	tx, err := m.pool.Begin(m.ctx)
	if err != nil {
		m.t.Fatal(err)
	}
	if _, err := tx.Exec(m.ctx, body); err != nil {
		_ = tx.Rollback(m.ctx)
		m.t.Fatalf("apply %s: %v", listingSalesUp, err)
	}
	var txTime time.Time
	if err := tx.QueryRow(m.ctx, `SELECT now()`).Scan(&txTime); err != nil {
		m.t.Fatal(err)
	}
	if err := tx.Commit(m.ctx); err != nil {
		m.t.Fatal(err)
	}
	return txTime
}

func (m *mdb) sold(listing string) (int64, bool) {
	m.t.Helper()
	rows, err := m.pool.Query(m.ctx, `SELECT sold_count FROM listing_sales WHERE listing_id = $1`, listing)
	if err != nil {
		m.t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, false
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		m.t.Fatal(err)
	}
	return n, true
}

func (m *mdb) wantSold(listing string, want int64) {
	m.t.Helper()
	got, ok := m.sold(listing)
	if !ok || got != want {
		m.t.Fatalf("sold_count[%s] = %d (row present %v), want %d", listing, got, ok, want)
	}
}

func (m *mdb) count(sql string, args ...any) int {
	m.t.Helper()
	var n int
	if err := m.pool.QueryRow(m.ctx, sql, args...).Scan(&n); err != nil {
		m.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (m *mdb) salesEvents() map[string]outbox.Row {
	m.t.Helper()
	rows, err := m.pool.Query(m.ctx, `SELECT event_id, aggregate_type, aggregate_id, event_type, payload
		FROM order_outbox_events WHERE event_type = $1`, salesEventType)
	if err != nil {
		m.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]outbox.Row{}
	for rows.Next() {
		var r outbox.Row
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Payload); err != nil {
			m.t.Fatal(err)
		}
		if _, dup := out[r.AggregateID]; dup {
			m.t.Fatalf("more than one ListingSalesChanged row for listing %s", r.AggregateID)
		}
		out[r.AggregateID] = r
	}
	if err := rows.Err(); err != nil {
		m.t.Fatal(err)
	}
	return out
}

func decodeSales(t *testing.T, r outbox.Row) (*eventsv1.EventEnvelope, *orderv1.ListingSalesChanged) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(r.Payload, &env); err != nil {
		t.Fatal(err)
	}
	var ev orderv1.ListingSalesChanged
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatal(err)
	}
	return &env, &ev
}

func (m *mdb) wantEventValue(listing string, want int64) {
	m.t.Helper()
	row, ok := m.salesEvents()[listing]
	if !ok {
		m.t.Fatalf("no ListingSalesChanged row for %s", listing)
	}
	_, ev := decodeSales(m.t, row)
	if ev.GetListingId() != listing || ev.GetSoldCount() != want {
		m.t.Fatalf("event for %s = {%s, %d}, want {%s, %d}", listing, ev.GetListingId(), ev.GetSoldCount(), listing, want)
	}
}

func (m *mdb) ledgerState(orderID, listing string) string {
	m.t.Helper()
	var s string
	rows, err := m.pool.Query(m.ctx, `SELECT state FROM listing_sales_orders WHERE order_id = $1 AND listing_id = $2`, orderID, listing)
	if err != nil {
		m.t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&s); err != nil {
			m.t.Fatal(err)
		}
	}
	return s
}

// Scenario: Existing paid orders are counted
func TestMigration0023_ExistingPaidOrdersAreCounted(t *testing.T) {
	m := newMigDB(t)
	m.order("o-paid", stPaid, mMomo, true)
	m.line("o-paid", "L", 2)
	m.order("o-completed", stCompleted, mMomo, true)
	m.line("o-completed", "L", 1)
	m.order("o-pending", stPending, mMomo, false)
	m.line("o-pending", "L", 4)

	m.runUp()

	m.wantSold("L", 3)
	m.wantEventValue("L", 3)
	if s := m.ledgerState("o-paid", "L"); s != "counted" {
		t.Fatalf("ledger of paid order = %q, want counted", s)
	}
	if s := m.ledgerState("o-pending", "L"); s == "counted" {
		t.Fatal("a pending order must not be counted")
	}
}

// Scenario: Orders paid before paid_at was recorded are counted
func TestMigration0023_OrdersPaidBeforePaidAtAreCounted(t *testing.T) {
	m := newMigDB(t)
	m.order("o-shipped", stShipped, mMomo, false)
	m.line("o-shipped", "L", 2)
	m.order("o-paid-legacy", stPaid, mMomo, false)
	m.line("o-paid-legacy", "L", 1)
	m.order("o-completed-legacy", stCompleted, mMomo, false)
	m.line("o-completed-legacy", "L", 1)
	m.order("o-cod-shipped", stShipped, mCOD, false)
	m.line("o-cod-shipped", "L", 5)
	m.order("o-unpaid-pending", stPending, mMomo, false)
	m.line("o-unpaid-pending", "L", 7)

	m.runUp()

	m.wantSold("L", 4)
	m.wantEventValue("L", 4)
	if s := m.ledgerState("o-cod-shipped", "L"); s == "counted" {
		t.Fatal("a COD order without paid_at has not been paid and must not be counted")
	}
}

// Scenario: Cancelled and refunded orders are excluded
func TestMigration0023_CancelledAndRefundedOrdersAreExcluded(t *testing.T) {
	m := newMigDB(t)
	m.order("o-cancelled", stCancelled, mMomo, true)
	m.line("o-cancelled", "L", 2)
	m.order("o-refunded", stCompleted, mMomo, true)
	m.line("o-refunded", "L", 1)
	m.ret("r-refunded", "o-refunded", returnRefunded)
	m.order("o-never-paid", stCancelled, mMomo, false)
	m.line("o-never-paid", "L", 3)

	m.order("o-approved-return", stCompleted, mMomo, true)
	m.line("o-approved-return", "L2", 4)
	m.ret("r-approved", "o-approved-return", returnApproved)
	m.order("o-pending-return", stCompleted, mMomo, true)
	m.line("o-pending-return", "L2", 1)
	m.ret("r-pending", "o-pending-return", returnPendingRet)

	m.runUp()

	m.wantSold("L", 0)
	m.wantEventValue("L", 0)
	if s := m.ledgerState("o-cancelled", "L"); s != "reversed" {
		t.Fatalf("paid-then-cancelled ledger = %q, want reversed", s)
	}
	if s := m.ledgerState("o-refunded", "L"); s != "reversed" {
		t.Fatalf("refunded ledger = %q, want reversed", s)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders WHERE listing_id = 'L' AND state = 'counted'`); n != 0 {
		t.Fatalf("%d counted rows for L, want 0", n)
	}
	m.wantSold("L2", 5)
	m.wantEventValue("L2", 5)
}

// Scenario: One event per listing
func TestMigration0023_OneEventPerListing(t *testing.T) {
	m := newMigDB(t)
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		m.order(id, stPaid, mMomo, true)
		m.line(id, "LA", 1)
	}
	m.order("b1", stPaid, mMomo, true)
	m.line("b1", "LB", 2)
	m.order("c1", stPending, mMomo, false)
	m.line("c1", "LC", 1)

	m.runUp()

	got := m.salesEvents()
	if len(got) != 3 {
		t.Fatalf("%d ListingSalesChanged rows, want 3", len(got))
	}
	if n := m.count(`SELECT count(*) FROM order_outbox_events`); n != 3 {
		t.Fatalf("%d outbox rows, want 3", n)
	}
	for listing, want := range map[string]int64{"LA": 5, "LB": 2, "LC": 0} {
		m.wantEventValue(listing, want)
		if got[listing].AggregateType == "Order" {
			t.Fatalf("listing event for %s is keyed as an Order aggregate", listing)
		}
	}
}

// Scenario: Running the migration body twice does not duplicate
func TestMigration0023_BackfillTwiceDoesNotDuplicate(t *testing.T) {
	m := newMigDB(t)
	m.order("o1", stPaid, mMomo, true)
	m.line("o1", "L", 2)
	m.order("o2", stCompleted, mMomo, true)
	m.line("o2", "L", 1)
	m.line("o2", "M", 6)

	m.runUp()
	first := m.salesEvents()
	ledgerRows := m.count(`SELECT count(*) FROM listing_sales_orders`)
	outboxRows := m.count(`SELECT count(*) FROM order_outbox_events`)

	m.runUp()

	m.wantSold("L", 3)
	m.wantSold("M", 6)
	if n := m.count(`SELECT count(*) FROM listing_sales`); n != 2 {
		t.Fatalf("listing_sales rows = %d, want 2", n)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != ledgerRows {
		t.Fatalf("ledger rows %d -> %d", ledgerRows, n)
	}
	if n := m.count(`SELECT count(*) FROM order_outbox_events`); n != outboxRows {
		t.Fatalf("outbox rows %d -> %d", outboxRows, n)
	}
	second := m.salesEvents()
	for listing, row := range first {
		if second[listing].EventID != row.EventID {
			t.Fatalf("event id of %s changed between runs", listing)
		}
	}
}

// Scenario: An empty database migrates cleanly
func TestMigration0023_EmptyDatabase(t *testing.T) {
	m := newMigDB(t)

	m.runUp()

	if n := m.count(`SELECT count(*) FROM listing_sales`); n != 0 {
		t.Fatalf("listing_sales has %d rows", n)
	}
	if n := m.count(`SELECT count(*) FROM listing_sales_orders`); n != 0 {
		t.Fatalf("listing_sales_orders has %d rows", n)
	}
	if n := m.count(`SELECT count(*) FROM order_outbox_events`); n != 0 {
		t.Fatalf("outbox has %d rows", n)
	}
}

// Scenario: Backfill events use a stable id and the commit time
func TestMigration0023_BackfillEventIDIsStableAndUsesCommitTime(t *testing.T) {
	seed := func(m *mdb, qty int) {
		m.order("o1", stPaid, mMomo, true)
		m.line("o1", "L", qty)
	}
	a := newMigDB(t)
	seed(a, 3)
	txTime := a.runUp()

	rowA := a.salesEvents()["L"]
	env, ev := decodeSales(t, rowA)
	if rowA.EventID == "" || env.GetEventId() != rowA.EventID {
		t.Fatalf("row event id %q, envelope event id %q", rowA.EventID, env.GetEventId())
	}
	if env.GetType() != salesEventType || ev.GetSoldCount() != 3 || ev.GetListingId() != "L" {
		t.Fatalf("envelope %q payload %v", env.GetType(), ev)
	}
	if got := ev.GetChangedAt().AsTime(); got.Sub(txTime).Abs() > time.Millisecond {
		t.Fatalf("changed_at = %v, want the migration transaction time %v", got, txTime)
	}

	b := newMigDB(t)
	seed(b, 3)
	b.runUp()
	if rowB := b.salesEvents()["L"]; rowB.EventID != rowA.EventID {
		t.Fatalf("backfill event id differs across runs for the same listing and count: %q vs %q", rowA.EventID, rowB.EventID)
	}

	c := newMigDB(t)
	seed(c, 4)
	c.runUp()
	if rowC := c.salesEvents()["L"]; rowC.EventID == rowA.EventID {
		t.Fatalf("backfill event id must depend on the sold count, both %q", rowA.EventID)
	}
}

// Scenario: A backfilled order cancelled later is subtracted once
func TestMigration0023_BackfilledOrderCancelledLaterIsSubtractedOnce(t *testing.T) {
	m := newMigDB(t)
	m.order("o1", stPaid, mMomo, true)
	m.line("o1", "L", 2)
	m.runUp()
	m.wantSold("L", 2)

	ob, err := outbox.NewStore(m.pool, "order_outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	orders := store.NewPostgresOrderRepository(m.pool, ob,
		store.WithPaidOutbox(events.BuildPaidOutboxRow),
		store.WithCancelledOutbox(events.BuildCancelledOutboxRow),
		store.WithListingSalesOutbox(events.BuildListingSalesChangedOutboxRow))
	returns := store.NewPostgresReturnRepository(m.pool, ob,
		store.WithReturnOutbox(events.BuildReturnRefundedOutboxRow),
		store.WithReturnListingSalesOutbox(events.BuildListingSalesChangedOutboxRow))

	if _, err := orders.UpdateOrderStatusFrom(m.ctx, "o1", store.OrderStatusCancelled,
		[]store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}, ""); err != nil {
		t.Fatal(err)
	}
	m.wantSold("L", 0)
	if s := m.ledgerState("o1", "L"); s != "reversed" {
		t.Fatalf("ledger = %q, want reversed", s)
	}

	ret, err := returns.CreateReturn(m.ctx, store.OrderReturn{
		OrderID: "o1", BuyerID: "buyer", SellerID: "seller", Reason: "x", RefundAmount: 10, Status: store.ReturnStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := returns.TransitionReturn(m.ctx, ret.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
		t.Fatal(err)
	}
	if _, err := returns.TransitionReturn(m.ctx, ret.ID, store.ReturnStatusApproved, store.ReturnStatusRefunded); err != nil {
		t.Fatal(err)
	}
	m.wantSold("L", 0)
	if n := m.count(`SELECT count(*) FROM order_outbox_events WHERE event_type = $1 AND aggregate_id = 'L'`, salesEventType); n != 2 {
		t.Fatalf("%d ListingSalesChanged rows for L (backfill and cancel), want 2", n)
	}
}
