package store_test

import (
	"errors"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

var (
	codFromShipped = []store.OrderStatus{store.OrderStatusShipped}
	codFromPaid    = []store.OrderStatus{store.OrderStatusPaid}
)

func codNewOrder(r *salesRig, lines ...line) store.Order {
	r.t.Helper()
	items := make([]store.OrderItem, len(lines))
	for i, l := range lines {
		items[i] = store.OrderItem{ListingID: l.listing, VariantID: "v" + string(rune('a'+i)), Quantity: l.qty, UnitPrice: 10}
	}
	o, err := r.orders.CreateOrder(r.ctx, store.Order{
		BuyerID: "b", SellerID: "s", TotalAmount: 10, Currency: "VND",
		PaymentMethod: store.CodPaymentMethod, Items: items,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return o
}

func codShip(r *salesRig, id string) {
	r.t.Helper()
	if _, err := r.orders.UpdateOrderStatusFrom(r.ctx, id, store.OrderStatusShipped, fromPending, "TRK"); err != nil {
		r.t.Fatalf("ship: %v", err)
	}
}

func codShippedOrder(r *salesRig, lines ...line) store.Order {
	r.t.Helper()
	o := codNewOrder(r, lines...)
	codShip(r, o.ID)
	return o
}

func codComplete(r *salesRig, id string, by orderv1.CompletedBy) error {
	_, err := r.orders.UpdateOrderStatusFrom(r.ctx, id, store.OrderStatusCompleted, codFromShipped, "",
		store.WithCompletedBy(int32(by)))
	return err
}

func codWantCompleted(r *salesRig, id string, by orderv1.CompletedBy) {
	r.t.Helper()
	if err := codComplete(r, id, by); err != nil {
		r.t.Fatalf("complete: %v", err)
	}
	got, err := r.orders.GetOrder(r.ctx, id)
	if err != nil {
		r.t.Fatal(err)
	}
	if got.Status != store.OrderStatusCompleted {
		r.t.Fatalf("status = %v, want COMPLETED", got.Status)
	}
}

func codWantLedger(r *salesRig, orderID, listing string, qty int32) {
	r.t.Helper()
	if state, q, ok := r.ledger(orderID, listing); !ok || state != "counted" || q != qty {
		r.t.Fatalf("ledger[%s,%s] = %q %d %v, want counted %d", orderID, listing, state, q, ok, qty)
	}
}

func TestCodSales_BuyerCompletionIsCounted(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	r.wantSold("L", 7, 7)
	codWantLedger(r, o.ID, "L", 2)
}

func TestCodSales_SellerCompletionIsCounted(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_SELLER)
	r.wantSold("L", 7, 7)
	codWantLedger(r, o.ID, "L", 2)
}

func TestCodSales_SweepCompletionIsCounted(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_SYSTEM)
	r.wantSold("L", 7, 7)
	codWantLedger(r, o.ID, "L", 2)
}

func TestCodSales_SeveralLinesOfOneListingAreSummed(t *testing.T) {
	r := newSalesRig(t)
	o := codShippedOrder(r, line{"L", 1}, line{"L", 3})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	r.wantSold("L", 4, 4)
	codWantLedger(r, o.ID, "L", 4)
}

func TestCodSales_OnlinePaidCompletionIsNotCountedAgain(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 3)
	o := r.paidOrder(line{"L", 2})
	r.wantSold("L", 5, 5)
	if _, err := r.orders.UpdateOrderStatusFrom(r.ctx, o.ID, store.OrderStatusShipped, codFromPaid, "TRK"); err != nil {
		t.Fatal(err)
	}
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	r.wantSold("L", 5, 5)
	codWantLedger(r, o.ID, "L", 2)
}

func TestCodSales_NotCountedBeforeCompletion(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	shipped := codNewOrder(r, line{"L", 2})
	codShip(r, shipped.ID)
	r.wantSold("L", 5)
	if _, _, ok := r.ledger(shipped.ID, "L"); ok {
		t.Fatal("a SHIPPED COD order must have no ledger row")
	}

	cancelled := codNewOrder(r, line{"L", 3})
	r.mustCancel(cancelled.ID)
	r.wantSold("L", 5)
	if _, _, ok := r.ledger(cancelled.ID, "L"); ok {
		t.Fatal("a COD order cancelled from PENDING must have no ledger row")
	}
}

func TestCodSales_PendingToCompletedIsRefused(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codNewOrder(r, line{"L", 2})
	if err := codComplete(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("complete from PENDING: %v, want ErrStatusConflict", err)
	}
	got, err := r.orders.GetOrder(r.ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.OrderStatusPending {
		t.Fatalf("status = %v, want PENDING", got.Status)
	}
	r.wantSold("L", 5)
	if _, _, ok := r.ledger(o.ID, "L"); ok {
		t.Fatal("a refused completion must leave no ledger row")
	}
}

func TestCodSales_RepeatedCompletionCountsOnce(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	if err := codComplete(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("second completion: %v, want ErrStatusConflict", err)
	}
	if err := codComplete(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_SYSTEM); !errors.Is(err, store.ErrStatusConflict) {
		t.Fatalf("sweep after completion: %v, want ErrStatusConflict", err)
	}
	r.wantSold("L", 7, 7)
}

func TestCodSales_ConcurrentCompletionCountsOnce(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	by := []orderv1.CompletedBy{
		orderv1.CompletedBy_COMPLETED_BY_BUYER,
		orderv1.CompletedBy_COMPLETED_BY_SELLER,
		orderv1.CompletedBy_COMPLETED_BY_SYSTEM,
	}
	errs := make([]error, len(by))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range by {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = codComplete(r, o.ID, by[i])
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, store.ErrStatusConflict):
			t.Fatalf("loser error = %v, want ErrStatusConflict", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d completions won, want exactly 1", wins)
	}
	r.wantSold("L", 7, 7)
}

func TestCodSales_OutboxFailureRollsBackCompletion(t *testing.T) {
	r := newSalesRig(t)
	failing := store.NewPostgresOrderRepository(r.pool, newOutbox(t, r.pool),
		store.WithListingSalesOutbox(failingListingSalesBuilder))
	o := codShippedOrder(r, line{"L", 2})
	if _, err := failing.UpdateOrderStatusFrom(r.ctx, o.ID, store.OrderStatusCompleted, codFromShipped, "",
		store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_BUYER))); err == nil {
		t.Fatal("completion must fail when the listing sales outbox row cannot be built")
	}
	got, err := r.orders.GetOrder(r.ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.OrderStatusShipped {
		t.Fatalf("status = %v, want SHIPPED after rollback", got.Status)
	}
	for _, table := range []string{"listing_sales", "listing_sales_orders", "order_outbox_events"} {
		if n := r.tableCount(table); n != 0 {
			t.Fatalf("%s holds %d rows after a rolled-back completion", table, n)
		}
	}
}

func TestCodSales_SeveralListingsEmitOneEventEach(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L2", 4)
	o := codShippedOrder(r, line{"L1", 2}, line{"L2", 1})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	r.wantSold("L1", 2, 2)
	r.wantSold("L2", 5, 5)
	if n := r.tableCount("order_outbox_events"); n != 2 {
		t.Fatalf("outbox holds %d rows, want 2 (one per listing)", n)
	}
}

func TestCodSales_ReturnRequestDoesNotChangeCount(t *testing.T) {
	r := newSalesRig(t)
	r.seedSoldCount("L", 5)
	o := codShippedOrder(r, line{"L", 2})
	codWantCompleted(r, o.ID, orderv1.CompletedBy_COMPLETED_BY_BUYER)
	r.wantSold("L", 7, 7)
	if _, err := r.returns.CreateReturn(r.ctx, store.OrderReturn{
		OrderID: o.ID, BuyerID: "b", SellerID: "s", Reason: "damaged", RefundAmount: 10, Status: store.ReturnStatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	r.wantSold("L", 7, 7)
	codWantLedger(r, o.ID, "L", 2)
}

func codBenchRepo(tb testing.TB, withSales bool) (*store.PostgresOrderRepository, *pgxpool.Pool) {
	tb.Helper()
	dsn := testkit.Schema(tb, testkit.SharedPostgres(tb))
	pool, err := pgxpool.New(tb.Context(), dsn)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(pool.Close)
	if err := migrate.Up(tb.Context(), dsn, os.DirFS("../../migrations")); err != nil {
		tb.Fatalf("migrate up: %v", err)
	}
	ob, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		tb.Fatal(err)
	}
	opts := []store.OrderRepoOption{}
	if withSales {
		opts = append(opts, store.WithListingSalesOutbox(testListingSalesBuilder))
	}
	return store.NewPostgresOrderRepository(pool, ob, opts...), pool
}

func codBenchRound(b *testing.B, withSales bool, orders int) time.Duration {
	b.Helper()
	repo, pool := codBenchRepo(b, withSales)
	ctx := b.Context()
	ids := make([]string, orders)
	for i := range ids {
		items := make([]store.OrderItem, 10)
		for j := range items {
			items[j] = store.OrderItem{ListingID: "L" + string(rune('a'+j)), VariantID: "v" + string(rune('a'+j)), Quantity: 1, UnitPrice: 10}
		}
		o, err := repo.CreateOrder(ctx, store.Order{
			BuyerID: "b", SellerID: "s", TotalAmount: 10, Currency: "VND",
			PaymentMethod: store.CodPaymentMethod, Items: items,
		})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusShipped, fromPending, "TRK"); err != nil {
			b.Fatal(err)
		}
		ids[i] = o.ID
	}
	lat := make([]time.Duration, orders)
	for i, id := range ids {
		t0 := time.Now()
		if _, err := repo.UpdateOrderStatusFrom(ctx, id, store.OrderStatusCompleted, codFromShipped, "",
			store.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_BUYER))); err != nil {
			b.Fatal(err)
		}
		lat[i] = time.Since(t0)
	}
	if withSales {
		var n int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(sold_count), 0) FROM listing_sales`).Scan(&n); err != nil {
			b.Fatal(err)
		}
		if n != int64(orders)*10 {
			b.Fatalf("sold_count total = %d after %d completions of 10 lines, want %d: completion is not being counted", n, orders, orders*10)
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return lat[(len(lat)*95+99)/100-1]
}

func codMedian(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

func BenchmarkCodCompletionLatency(b *testing.B) {
	const (
		orders = 200
		rounds = 3
		budget = 5 * time.Millisecond
	)
	for range b.N {
		base := make([]time.Duration, rounds)
		with := make([]time.Duration, rounds)
		for i := 0; i < rounds; i++ {
			base[i] = codBenchRound(b, false, orders)
			with[i] = codBenchRound(b, true, orders)
		}
		mb, mw := codMedian(base), codMedian(with)
		b.ReportMetric(float64(mb.Microseconds())/1000, "baseline_p95_ms")
		b.ReportMetric(float64(mw.Microseconds())/1000, "counted_p95_ms")
		if mw-mb > budget {
			b.Fatalf("median p95 %v exceeds baseline %v by more than %v", mw, mb, budget)
		}
	}
}
