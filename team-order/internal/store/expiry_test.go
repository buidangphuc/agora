package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

// expiryRepo is the order repository as the expiry pass uses it. FindExpired
// returns up to limit Pending, non-COD orders whose payment_due_at is strictly
// before now, earliest deadline first.
type expiryRepo interface {
	store.OrderRepository
	FindExpired(ctx context.Context, now time.Time, limit int) ([]store.Order, error)
}

var expired = []store.OrderStatus{store.OrderStatusPending}

func expire(ctx context.Context, repo store.OrderRepository, id string) (store.Order, error) {
	return repo.UpdateOrderStatusFrom(ctx, id, store.OrderStatusCancelled, expired, "",
		store.WithCancelReason(int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED)))
}

// runExpiryContract: FindExpired selects exactly the Pending online orders past
// their deadline (strictly), and the system claim stores PAYMENT_EXPIRED.
func runExpiryContract(t *testing.T, repo expiryRepo, due time.Time) {
	ctx := context.Background()
	drainExpired(t, repo, due.Add(72*time.Hour))
	find := func(now time.Time) map[string]bool {
		t.Helper()
		got, err := repo.FindExpired(ctx, now, 100)
		if err != nil {
			t.Fatal(err)
		}
		return ids(got)
	}

	t.Run("deadline is exclusive", func(t *testing.T) { // Order expiry: Boundary at the deadline keeps the order payable
		d := due
		o := newUnpaid(t, repo, momo, &d)
		if find(d.Add(-time.Second))[o.ID] || find(d)[o.ID] {
			t.Fatal("before or exactly at the deadline the order must not be expired")
		}
		if !find(d.Add(time.Second))[o.ID] {
			t.Fatal("one second after the deadline the order must be expired")
		}
	})

	t.Run("cod paid cancelled and deadline-less orders are never selected", func(t *testing.T) { // Order expiry: A COD order is never expired; An order without a deadline is never expired; A paid order is never expired
		d := due.Add(24 * time.Hour)
		now := d.Add(time.Hour)
		paid := newUnpaid(t, repo, momo, &d)
		if _, err := repo.UpdateOrderStatusFrom(ctx, paid.ID, store.OrderStatusPaid,
			[]store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
			t.Fatal(err)
		}
		gone := newUnpaid(t, repo, momo, &d)
		if _, err := expire(ctx, repo, gone.ID); err != nil {
			t.Fatal(err)
		}
		cod := newUnpaid(t, repo, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, &d)
		noDue := newUnpaid(t, repo, momo, nil)
		live := newUnpaid(t, repo, momo, &d)
		got := find(now)
		if got[paid.ID] || got[gone.ID] || got[cod.ID] || got[noDue.ID] {
			t.Fatalf("must not be selected: paid=%v cancelled=%v cod=%v noDue=%v", got[paid.ID], got[gone.ID], got[cod.ID], got[noDue.ID])
		}
		if !got[live.ID] {
			t.Fatal("the unpaid online order past its deadline must be selected")
		}
	})

	t.Run("claim stores PAYMENT_EXPIRED and removes the order from the pass", func(t *testing.T) { // Order expiry: Order unpaid past its deadline is cancelled and its stock released
		d := due.Add(48 * time.Hour)
		o := newUnpaid(t, repo, momo, &d)
		got, err := expire(ctx, repo, o.ID)
		if err != nil || got.Status != store.OrderStatusCancelled ||
			got.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) {
			t.Fatalf("claim: %v %+v", err, got)
		}
		if find(d.Add(time.Hour))[o.ID] {
			t.Fatal("a cancelled order is no longer expirable")
		}
	})
}

// drainExpired claims every Pending online order already past upTo, so foreign rows
// left by other tests on a shared Postgres cannot crowd this test's rows out of the
// bounded (limit 100, earliest first) FindExpired window.
func drainExpired(t *testing.T, repo expiryRepo, upTo time.Time) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		got, err := repo.FindExpired(ctx, upTo, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			return
		}
		for _, o := range got {
			if _, err := expire(ctx, repo, o.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Fatal("could not drain foreign expirable orders")
}

func runFindExpiredBatchBound(t *testing.T, repo expiryRepo, total int, due time.Time) {
	ctx := context.Background()
	own := map[string]bool{}
	for i := 0; i < total; i++ {
		d := due
		own[newUnpaid(t, repo, momo, &d).ID] = true
	}
	// The pass looks at every Pending online order in the database, so a shared
	// Postgres may hold foreign rows (other tests); they only add to a tick and are
	// drained like any expired order. Assert the bound and that our rows all drain.
	now := due.Add(time.Hour)
	ownLeft := total
	for tick := 1; ownLeft > 0; tick++ {
		if tick > total+10 {
			t.Fatalf("expiry pass did not drain %d own orders", ownLeft)
		}
		got, err := repo.FindExpired(ctx, now, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > 100 {
			t.Fatalf("tick %d: bounded at 100, got %d", tick, len(got))
		}
		if tick == 1 && len(got) != 100 {
			t.Fatalf("tick 1: %d pending orders exist, want a full batch of 100, got %d", total, len(got))
		}
		if len(got) == 0 {
			t.Fatalf("tick %d: %d own orders still pending but the pass returned none", tick, ownLeft)
		}
		for _, o := range got {
			if _, err := expire(ctx, repo, o.ID); err != nil {
				t.Fatal(err)
			}
			if own[o.ID] {
				delete(own, o.ID)
				ownLeft--
			}
		}
	}
	got, err := repo.FindExpired(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got {
		if own[o.ID] {
			t.Fatalf("own order %s left after all ticks", o.ID)
		}
	}
}

func TestFindExpiredContract_InMemory(t *testing.T) {
	repo := fake.NewInMemoryOrderRepository()
	runExpiryContract(t, repo, time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC))
}

func TestFindExpiredContract_Postgres(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithCancelledOutbox(BuildCancelledOutboxRow))
	runExpiryContract(t, repo, uniqueDue())
}

// Scenario: Order expiry: An expiry pass handles at most 100 orders per tick
func TestFindExpiredBatchBound(t *testing.T) {
	repo := fake.NewInMemoryOrderRepository()
	runFindExpiredBatchBound(t, repo, 250, time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC))
}

func TestFindExpiredBatchBound_Postgres(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithCancelledOutbox(BuildCancelledOutboxRow))
	runFindExpiredBatchBound(t, repo, 105, uniqueDue())
}

// raceOutcome: PaymentSettled (to Paid) and the expiry claim race on one Pending
// order past its deadline; exactly one CAS wins and the outbox matches the winner.
func runExpiryVersusPayment(t *testing.T, repo store.OrderRepository, due time.Time, cancelledRows func(id string) int, paidRows func(id string) int) {
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		d := due.Add(time.Duration(i) * time.Minute)
		o := newUnpaid(t, repo, momo, &d)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var paidErr, expErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, paidErr = repo.UpdateOrderStatusFrom(ctx, o.ID, store.OrderStatusPaid,
				[]store.OrderStatus{store.OrderStatusPending}, "")
		}()
		go func() { defer wg.Done(); <-start; _, expErr = expire(ctx, repo, o.ID) }()
		close(start)
		wg.Wait()
		if (paidErr == nil) == (expErr == nil) {
			t.Fatalf("round %d: exactly one CAS must win, paidErr=%v expiryErr=%v", i, paidErr, expErr)
		}
		stored, _ := repo.GetOrder(ctx, o.ID)
		switch {
		case paidErr == nil:
			if stored.Status != store.OrderStatusPaid || cancelledRows(o.ID) != 0 || paidRows(o.ID) != 1 {
				t.Fatalf("round %d paid won: status=%v cancelled=%d paid=%d", i, stored.Status, cancelledRows(o.ID), paidRows(o.ID))
			}
		default:
			if stored.Status != store.OrderStatusCancelled ||
				stored.CancelReason != int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED) ||
				cancelledRows(o.ID) != 1 || paidRows(o.ID) != 0 {
				t.Fatalf("round %d expiry won: status=%v reason=%d cancelled=%d paid=%d", i, stored.Status,
					stored.CancelReason, cancelledRows(o.ID), paidRows(o.ID))
			}
		}
	}
}

// Scenario: Order expiry: Payment and expiry racing produce one outcome
func TestExpiryVersusPaymentSingleWinner(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool), store.WithPaidOutbox(BuildPaidOutboxRow),
		store.WithCancelledOutbox(BuildCancelledOutboxRow))
	count := func(typ string) func(string) int {
		return func(id string) int {
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1 AND event_type = $2`, id, typ).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	runExpiryVersusPayment(t, repo, uniqueDue(), count(OrderCancelledEventType), count(OrderPaidEventType))
}
