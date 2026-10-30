package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/buidangphuc/team-order/internal/store"
)

type reapprover interface {
	ReapproveReturn(ctx context.Context, id string, orderTotal int64) (store.OrderReturn, error)
}

func asReapprover(t *testing.T, r store.ReturnRepository) reapprover {
	t.Helper()
	ra, ok := r.(reapprover)
	if !ok {
		t.Fatalf("%T does not implement ReapproveReturn(ctx, id, orderTotal) (OrderReturn, error)", r)
	}
	return ra
}

func rejectedReturn(ctx context.Context, t *testing.T, f returnFixture, o store.Order, amount int64) store.OrderReturn {
	t.Helper()
	r := newReturn(ctx, t, f, o, amount)
	if _, err := f.returns.TransitionReturn(ctx, r.ID, store.ReturnStatusPending, store.ReturnStatusRejected); err != nil {
		t.Fatal(err)
	}
	return r
}

func runReapproveContract(t *testing.T, f returnFixture) {
	ctx := context.Background()
	ra := asReapprover(t, f.returns)
	const total = 1000000

	t.Run("TestReapproveReturnFitsBoundary: Dispute refund: Rejected return that still fits is re-approved", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		target := rejectedReturn(ctx, t, f, o, 400000)
		other := newReturn(ctx, t, f, o, 600000)
		got, err := ra.ReapproveReturn(ctx, target.ID, o.TotalAmount)
		if err != nil {
			t.Fatalf("want success at the exact boundary, got %v", err)
		}
		if got.Status != store.ReturnStatusApproved || got.RefundAmount != 400000 {
			t.Fatalf("returned row: %+v", got)
		}
		if st, _ := f.returns.GetReturn(ctx, target.ID); st.Status != store.ReturnStatusApproved || st.RefundAmount != 400000 {
			t.Fatalf("persisted: %+v", st)
		}
		if st, _ := f.returns.GetReturn(ctx, other.ID); st.Status != store.ReturnStatusPending {
			t.Fatalf("other return must be untouched: %v", st.Status)
		}
		if n := f.rows(t, o.ID, target.ID); f.rows != nil && n != 0 {
			t.Fatalf("re-approval must write no outbox row, got %d", n)
		}
	})

	t.Run("TestReapproveReturnExceedsRemainderStaysRejected: Dispute refund: Rejected return that no longer fits stays rejected", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		target := rejectedReturn(ctx, t, f, o, 400000)
		newReturn(ctx, t, f, o, 700000)
		if _, err := ra.ReapproveReturn(ctx, target.ID, o.TotalAmount); !errors.Is(err, store.ErrReturnExceedsRemainder) {
			t.Fatalf("want ErrReturnExceedsRemainder, got %v", err)
		}
		if st, _ := f.returns.GetReturn(ctx, target.ID); st.Status != store.ReturnStatusRejected {
			t.Fatalf("must stay REJECTED, got %v", st.Status)
		}
	})

	t.Run("TestReapproveReturnOneOverBoundary: Dispute refund: Rejected return that no longer fits stays rejected", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		target := rejectedReturn(ctx, t, f, o, 400000)
		newReturn(ctx, t, f, o, 600001)
		if _, err := ra.ReapproveReturn(ctx, target.ID, o.TotalAmount); !errors.Is(err, store.ErrReturnExceedsRemainder) {
			t.Fatalf("want ErrReturnExceedsRemainder, got %v", err)
		}
	})

	t.Run("TestReapproveReturnIgnoresOtherRejected: Dispute refund: Rejected return that still fits is re-approved", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		target := rejectedReturn(ctx, t, f, o, 400000)
		rejectedReturn(ctx, t, f, o, 900000)
		if _, err := ra.ReapproveReturn(ctx, target.ID, o.TotalAmount); err != nil {
			t.Fatalf("other rejected returns must not count, got %v", err)
		}
	})

	t.Run("TestReapproveReturnNotRejectedConflicts: Dispute refund: Rejected return that no longer fits stays rejected", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		pending := newReturn(ctx, t, f, o, 100000)
		if _, err := ra.ReapproveReturn(ctx, pending.ID, o.TotalAmount); !errors.Is(err, store.ErrReturnStatusConflict) {
			t.Fatalf("want ErrReturnStatusConflict, got %v", err)
		}
		if st, _ := f.returns.GetReturn(ctx, pending.ID); st.Status != store.ReturnStatusPending {
			t.Fatalf("status must be unchanged: %v", st.Status)
		}
		if _, err := ra.ReapproveReturn(ctx, "missing-"+pending.ID, o.TotalAmount); !errors.Is(err, store.ErrReturnNotFound) {
			t.Fatalf("want ErrReturnNotFound, got %v", err)
		}
	})

	t.Run("TestReapproveReturnConcurrentSerialised: Dispute refund: Two returns cannot both be re-approved past the total", func(t *testing.T) {
		o := newReturnOrder(ctx, t, f, total)
		a := rejectedReturn(ctx, t, f, o, 400000)
		b := rejectedReturn(ctx, t, f, o, 400000)
		newReturn(ctx, t, f, o, 300000)
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, over := 0, 0
		for _, id := range []string{a.ID, b.ID} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				_, err := ra.ReapproveReturn(ctx, id, o.TotalAmount)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, store.ErrReturnExceedsRemainder):
					over++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(id)
		}
		wg.Wait()
		if ok != 1 || over != 1 {
			t.Fatalf("ok=%d over=%d", ok, over)
		}
		approved := 0
		list, _ := f.returns.ListReturnsByOrder(ctx, o.ID)
		var sum int64
		for _, r := range list {
			if r.Status != store.ReturnStatusRejected {
				sum += r.RefundAmount
			}
			if r.Status == store.ReturnStatusApproved {
				approved++
			}
		}
		if approved != 1 || sum > total {
			t.Fatalf("approved=%d non-rejected sum=%d", approved, sum)
		}
	})
}

func TestReapproveReturn_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	orders := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
	returns := store.NewPostgresReturnRepository(pool, newOutbox(t, pool), store.WithReturnOutbox(fakeReturnOutbox))
	rows := func(t *testing.T, orderID, returnID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1 AND event_id = $2`,
			orderID, "rr-"+returnID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	runReapproveContract(t, returnFixture{orders: orders, returns: returns, rows: rows})
}
