package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

const listingSalesChangedType = "platform.order.v1.ListingSalesChanged"

func testListingSalesBuilder(c store.ListingSalesChange) (outbox.Row, error) {
	id := fmt.Sprintf("listing-sales:%s:%d:%s", c.ListingID, c.SoldCount, c.SourceEventID)
	payload, err := proto.Marshal(&orderv1.ListingSalesChanged{
		ListingId: c.ListingID,
		SoldCount: c.SoldCount,
		ChangedAt: timestamppb.New(c.ChangedAt),
	})
	if err != nil {
		return outbox.Row{}, err
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: id, Type: listingSalesChangedType, Payload: payload})
	if err != nil {
		return outbox.Row{}, err
	}
	return outbox.Row{
		EventID:       id,
		AggregateType: "Listing",
		AggregateID:   c.ListingID,
		EventType:     listingSalesChangedType,
		Payload:       value,
	}, nil
}

func failingListingSalesBuilder(store.ListingSalesChange) (outbox.Row, error) {
	return outbox.Row{}, errors.New("builder failure")
}

type salesRig struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	orders  *store.PostgresOrderRepository
	returns *store.PostgresReturnRepository
}

func newSalesRig(t *testing.T) *salesRig {
	t.Helper()
	pool := pgPool(t)
	ob := newOutbox(t, pool)
	return &salesRig{
		t:    t,
		ctx:  context.Background(),
		pool: pool,
		orders: store.NewPostgresOrderRepository(pool, ob,
			store.WithPaidOutbox(BuildPaidOutboxRow),
			store.WithCancelledOutbox(BuildCancelledOutboxRow),
			store.WithListingSalesOutbox(testListingSalesBuilder)),
		returns: store.NewPostgresReturnRepository(pool, ob,
			store.WithReturnOutbox(BuildReturnRefundedOutboxRow),
			store.WithReturnListingSalesOutbox(testListingSalesBuilder)),
	}
}

type line struct {
	listing string
	qty     int32
}

func (r *salesRig) newOrder(lines ...line) store.Order {
	r.t.Helper()
	items := make([]store.OrderItem, len(lines))
	for i, l := range lines {
		items[i] = store.OrderItem{ListingID: l.listing, VariantID: fmt.Sprintf("v%d", i), Quantity: l.qty, UnitPrice: 10}
	}
	o, err := r.orders.CreateOrder(r.ctx, store.Order{
		BuyerID: "b", SellerID: "s", TotalAmount: 10, Currency: "VND", Items: items,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return o
}

func (r *salesRig) pay(id string) {
	r.t.Helper()
	if _, err := r.orders.UpdateOrderStatusFrom(r.ctx, id, store.OrderStatusPaid, fromPending, ""); err != nil {
		r.t.Fatalf("pay: %v", err)
	}
}

func (r *salesRig) paidOrder(lines ...line) store.Order {
	r.t.Helper()
	o := r.newOrder(lines...)
	r.pay(o.ID)
	return o
}

func (r *salesRig) cancel(id string) error {
	_, err := r.orders.UpdateOrderStatusFrom(r.ctx, id, store.OrderStatusCancelled, fromPendingPaid, "")
	return err
}

func (r *salesRig) mustCancel(id string) {
	r.t.Helper()
	if err := r.cancel(id); err != nil {
		r.t.Fatalf("cancel: %v", err)
	}
}

func (r *salesRig) refund(order store.Order) error {
	r.t.Helper()
	ret, err := r.returns.CreateReturn(r.ctx, store.OrderReturn{
		OrderID: order.ID, BuyerID: "b", SellerID: "s", Reason: "damaged", RefundAmount: 10, Status: store.ReturnStatusPending,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := r.returns.TransitionReturn(r.ctx, ret.ID, store.ReturnStatusPending, store.ReturnStatusApproved); err != nil {
		r.t.Fatal(err)
	}
	_, err = r.returns.TransitionReturn(r.ctx, ret.ID, store.ReturnStatusApproved, store.ReturnStatusRefunded)
	return err
}

func (r *salesRig) soldCount(listing string) int64 {
	r.t.Helper()
	var n int64
	err := r.pool.QueryRow(r.ctx, `SELECT sold_count FROM listing_sales WHERE listing_id = $1`, listing).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *salesRig) seedSoldCount(listing string, n int64) {
	r.t.Helper()
	if _, err := r.pool.Exec(r.ctx, `INSERT INTO listing_sales (listing_id, sold_count) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET sold_count = EXCLUDED.sold_count`, listing, n); err != nil {
		r.t.Fatal(err)
	}
}

func (r *salesRig) ledger(orderID, listing string) (string, int32, bool) {
	r.t.Helper()
	var state string
	var qty int32
	err := r.pool.QueryRow(r.ctx, `SELECT state, quantity FROM listing_sales_orders WHERE order_id = $1 AND listing_id = $2`,
		orderID, listing).Scan(&state, &qty)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return state, qty, true
}

func (r *salesRig) emitted(listing string) []int64 {
	r.t.Helper()
	rows, err := r.pool.Query(r.ctx, `SELECT payload FROM order_outbox_events
		WHERE aggregate_id = $1 AND event_type = $2 ORDER BY created_at, event_id`, listing, listingSalesChangedType)
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var p []byte
		if err := rows.Scan(&p); err != nil {
			r.t.Fatal(err)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(p, &env); err != nil {
			r.t.Fatal(err)
		}
		var ev orderv1.ListingSalesChanged
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			r.t.Fatal(err)
		}
		if ev.GetListingId() != listing {
			r.t.Fatalf("event for %q carries listing %q", listing, ev.GetListingId())
		}
		out = append(out, ev.GetSoldCount())
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *salesRig) tableCount(table string) int {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(r.ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *salesRig) wantSold(listing string, want int64, wantEvents ...int64) {
	r.t.Helper()
	if got := r.soldCount(listing); got != want {
		r.t.Fatalf("sold_count[%s] = %d, want %d", listing, got, want)
	}
	if got := r.emitted(listing); !equalInts(got, wantEvents) {
		r.t.Fatalf("ListingSalesChanged[%s] = %v, want %v", listing, got, wantEvents)
	}
}

// Scenario: Payment adds the order quantity
func TestListingSales_PaymentAddsOrderQuantity(t *testing.T) {
	r := newSalesRig(t)
	o := r.paidOrder(line{"L", 2})
	r.wantSold("L", 2, 2)
	if state, qty, ok := r.ledger(o.ID, "L"); !ok || state != "counted" || qty != 2 {
		t.Fatalf("ledger = %q %d %v, want counted 2", state, qty, ok)
	}
}

// Scenario: A payment emits the new absolute value
func TestListingSales_PaymentEmitsAbsoluteValue(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 3})
	r.paidOrder(line{"L", 2})
	r.wantSold("L", 5, 3, 5)
}

// Scenario: Cancel from PAID subtracts the order quantity
func TestListingSales_CancelFromPaidSubtractsOrderQuantity(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 3})
	o := r.paidOrder(line{"L", 2})
	r.mustCancel(o.ID)
	r.wantSold("L", 3, 3, 5, 3)
	if state, _, _ := r.ledger(o.ID, "L"); state != "reversed" {
		t.Fatalf("ledger state = %q, want reversed", state)
	}
}

// Scenario: Cancel before PAID changes nothing
func TestListingSales_CancelFromPendingChangesNothing(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 5})
	o := r.newOrder(line{"L", 2})
	r.mustCancel(o.ID)
	r.wantSold("L", 5, 5)
	if _, _, ok := r.ledger(o.ID, "L"); ok {
		t.Fatal("a never-paid order must have no ledger row")
	}
}

// Scenario: Refunded return subtracts the order quantity
func TestListingSales_RefundedReturnSubtractsOrderQuantity(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 3})
	o := r.paidOrder(line{"L", 2})
	if err := r.refund(o); err != nil {
		t.Fatal(err)
	}
	r.wantSold("L", 3, 3, 5, 3)
	if state, _, _ := r.ledger(o.ID, "L"); state != "reversed" {
		t.Fatalf("ledger state = %q, want reversed", state)
	}
}

// Scenario: A repeated or redelivered transition changes nothing
func TestListingSales_RepeatedTransitionsChangeNothing(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 3})
	o := r.paidOrder(line{"L", 2})
	r.mustCancel(o.ID)
	if err := r.cancel(o.ID); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("second cancel: %v, want ErrStatusConflict", err)
	}
	if _, err := r.orders.UpdateOrderStatusFrom(r.ctx, o.ID, store.OrderStatusPaid, fromPending, ""); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("pay after cancel: %v, want ErrStatusConflict", err)
	}
	r.wantSold("L", 3, 3, 5, 3)

	p := r.paidOrder(line{"M", 4})
	if _, err := r.orders.UpdateOrderStatusFrom(r.ctx, p.ID, store.OrderStatusPaid, fromPending, ""); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("second pay: %v, want ErrStatusConflict", err)
	}
	r.wantSold("M", 4, 4)
}

// Scenario: A cancel and a refund of the same order subtract once
func TestListingSales_CancelThenRefundSubtractsOnce(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L", 3})
	o := r.paidOrder(line{"L", 2})
	r.mustCancel(o.ID)
	if err := r.refund(o); err != nil {
		t.Fatal(err)
	}
	r.wantSold("L", 3, 3, 5, 3)
}

// Scenario: The count never goes below 0
func TestListingSales_FloorsAtZeroWithSeededCount(t *testing.T) {
	r := newSalesRig(t)
	o := r.paidOrder(line{"L", 2})
	r.seedSoldCount("L", 1)
	r.mustCancel(o.ID)
	if got := r.soldCount("L"); got != 0 {
		t.Fatalf("sold_count = %d, want 0", got)
	}
	for _, v := range r.emitted("L") {
		if v < 0 {
			t.Fatalf("emitted negative sold_count %d", v)
		}
	}
	if _, err := r.pool.Exec(r.ctx, `UPDATE listing_sales SET sold_count = -1 WHERE listing_id = 'L'`); err == nil {
		t.Fatal("a negative sold_count must be rejected by the database")
	}
}

// Scenario: A refund or cancel with no counted ledger rows changes nothing
func TestListingSales_NoCountedRowsChangesNothing(t *testing.T) {
	r := newSalesRig(t)
	cancelled := r.paidOrder(line{"L", 2})
	refunded := r.paidOrder(line{"L", 2})
	if _, err := r.pool.Exec(r.ctx, `DELETE FROM listing_sales_orders`); err != nil {
		t.Fatal(err)
	}
	r.seedSoldCount("L", 5)
	before := r.emitted("L")

	r.mustCancel(cancelled.ID)
	if err := r.refund(refunded); err != nil {
		t.Fatal(err)
	}
	r.wantSold("L", 5, before...)
}

// Scenario: An order with several lines of the same listing is summed
func TestListingSales_SeveralLinesOfOneListingAreSummed(t *testing.T) {
	r := newSalesRig(t)
	o := r.paidOrder(line{"L", 1}, line{"L", 3})
	r.wantSold("L", 4, 4)
	if state, qty, ok := r.ledger(o.ID, "L"); !ok || state != "counted" || qty != 4 {
		t.Fatalf("ledger = %q %d %v, want counted 4", state, qty, ok)
	}
	r.mustCancel(o.ID)
	r.wantSold("L", 0, 4, 0)
}

// Scenario: A multi-listing order emits one event per listing
func TestListingSales_MultiListingOrderEmitsOneEventPerListing(t *testing.T) {
	r := newSalesRig(t)
	r.paidOrder(line{"L2", 1})
	o := r.paidOrder(line{"L1", 2}, line{"L2", 3})
	r.wantSold("L1", 2, 2)
	r.wantSold("L2", 4, 1, 4)
	r.mustCancel(o.ID)
	r.wantSold("L1", 0, 2, 0)
	r.wantSold("L2", 1, 1, 4, 1)
}

// Scenario: The event commits or rolls back with the state
func TestListingSales_PaidRollbackLeavesNoRows(t *testing.T) {
	r := newSalesRig(t)
	ob := newOutbox(t, r.pool)
	failing := store.NewPostgresOrderRepository(r.pool, ob,
		store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithListingSalesOutbox(failingListingSalesBuilder))
	o := r.newOrder(line{"L", 2})
	if _, err := failing.UpdateOrderStatusFrom(r.ctx, o.ID, store.OrderStatusPaid, fromPending, ""); err == nil {
		t.Fatal("pay must fail when the listing sales outbox row cannot be built")
	}
	got, err := r.orders.GetOrder(r.ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.OrderStatusPending {
		t.Fatalf("status = %v, want PENDING after rollback", got.Status)
	}
	for _, table := range []string{"listing_sales", "listing_sales_orders", "order_outbox_events"} {
		if n := r.tableCount(table); n != 0 {
			t.Fatalf("%s holds %d rows after a rolled-back pay", table, n)
		}
	}
}
