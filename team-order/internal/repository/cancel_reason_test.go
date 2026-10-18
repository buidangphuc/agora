package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
)

const (
	reasonChangedMind = int32(orderv1.CancelReason_CANCEL_REASON_CHANGED_MIND)
	reasonExpired     = int32(orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED)
)

// runCancelReasonContract: the claim that wins a move to Cancelled stores its
// reason on the row and on the OrderCancelled fact; losers and other moves never
// touch it.
func runCancelReasonContract(t *testing.T, repo repository.OrderRepository, rows cancelRowsFn) {
	ctx := context.Background()

	t.Run("won cancel stores the reason on the row and the event", func(t *testing.T) { // Order expiry: Buyer cancel stores the chosen reason
		o := newCancelOrder(t, repo)
		got, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, "",
			repository.WithCancelReason(reasonChangedMind))
		if err != nil || got.CancelReason != reasonChangedMind {
			t.Fatalf("claim: %v %+v", err, got)
		}
		stored, _ := repo.GetOrder(ctx, o.ID)
		if stored.CancelReason != reasonChangedMind {
			t.Fatalf("stored reason %d", stored.CancelReason)
		}
		r := rows(t, o.ID)
		if len(r) != 1 || int32(r[0].GetReasonCode()) != reasonChangedMind {
			t.Fatalf("event: %+v", r)
		}
	})

	t.Run("no reason option stores unspecified", func(t *testing.T) { // Order expiry: Cancel without a reason code still succeeds
		o := newCancelOrder(t, repo)
		got, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, "")
		if err != nil || got.CancelReason != 0 {
			t.Fatalf("claim: %v %+v", err, got)
		}
		r := rows(t, o.ID)
		if len(r) != 1 || r[0].GetReasonCode() != orderv1.CancelReason_CANCEL_REASON_UNSPECIFIED {
			t.Fatalf("event: %+v", r)
		}
	})

	t.Run("reason is ignored on moves other than Cancelled", func(t *testing.T) {
		o := newCancelOrder(t, repo)
		got, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, "",
			repository.WithCancelReason(reasonChangedMind))
		if err != nil || got.CancelReason != 0 {
			t.Fatalf("pay: %v %+v", err, got)
		}
		if stored, _ := repo.GetOrder(ctx, o.ID); stored.CancelReason != 0 {
			t.Fatalf("stored reason %d on a Paid order", stored.CancelReason)
		}
	})

	t.Run("refused cancel of a shipped order leaves the reason unset", func(t *testing.T) { // Order expiry: Cancel of a shipped order is refused
		o := newCancelOrder(t, repo)
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, fromPending, "TRK"); err != nil {
			t.Fatal(err)
		}
		_, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPendingPaid, "",
			repository.WithCancelReason(reasonChangedMind))
		if !errors.Is(err, repository.ErrStatusConflict) {
			t.Fatalf("want ErrStatusConflict, got %v", err)
		}
		if stored, _ := repo.GetOrder(ctx, o.ID); stored.CancelReason != 0 || stored.Status != repository.OrderStatusShipped {
			t.Fatalf("order must be untouched: %+v", stored)
		}
		if n := len(rows(t, o.ID)); n != 0 {
			t.Fatalf("no event for a refused cancel, got %d", n)
		}
	})

	t.Run("system actor set cannot cancel a Paid order", func(t *testing.T) { // Order expiry: System may cancel only from Pending
		o := newCancelOrder(t, repo)
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, fromPending, ""); err != nil {
			t.Fatal(err)
		}
		_, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, fromPending, "",
			repository.WithCancelReason(reasonExpired))
		if !errors.Is(err, repository.ErrStatusConflict) {
			t.Fatalf("want ErrStatusConflict, got %v", err)
		}
		if stored, _ := repo.GetOrder(ctx, o.ID); stored.Status != repository.OrderStatusPaid || stored.CancelReason != 0 {
			t.Fatalf("order must be untouched: %+v", stored)
		}
	})

	t.Run("buyer cancel and expiry race: one reason", func(t *testing.T) { // Order expiry: Concurrent buyer cancel and expiry keep one reason
		for i := 0; i < 10; i++ {
			o := newCancelOrder(t, repo)
			var wg sync.WaitGroup
			var mu sync.Mutex
			winners := map[int32]bool{}
			start := make(chan struct{})
			try := func(from []repository.OrderStatus, reason int32) {
				defer wg.Done()
				<-start
				_, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCancelled, from, "", repository.WithCancelReason(reason))
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					winners[reason] = true
				case errors.Is(err, repository.ErrStatusConflict):
				default:
					t.Errorf("unexpected: %v", err)
				}
			}
			wg.Add(2)
			go try(fromPendingPaid, reasonChangedMind)
			go try(fromPending, reasonExpired)
			close(start)
			wg.Wait()
			if len(winners) != 1 {
				t.Fatalf("want exactly one winner, got %v", winners)
			}
			stored, _ := repo.GetOrder(ctx, o.ID)
			r := rows(t, o.ID)
			if len(r) != 1 {
				t.Fatalf("want exactly one OrderCancelled, got %d", len(r))
			}
			if !winners[stored.CancelReason] || int32(r[0].GetReasonCode()) != stored.CancelReason {
				t.Fatalf("stored reason %d, event reason %v, winners %v", stored.CancelReason, r[0].GetReasonCode(), winners)
			}
		}
	})
}

// Order expiry: Concurrent buyer cancel and expiry keep one reason (named by the spec).
func TestCancelReasonMatchesClaimWinner(t *testing.T) {
	outbox := repository.NewInMemoryOutboxRepository()
	repo := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow), repository.WithInMemoryOutbox(outbox))
	runCancelReasonContract(t, repo, func(t *testing.T, id string) []*orderv1.OrderCancelled {
		var payloads [][]byte
		for _, r := range outbox.EnqueuedRows() {
			if r.AggregateID == id {
				payloads = append(payloads, r.Payload)
			}
		}
		return decodeCancelPayloads(t, payloads)
	})
}

func TestCancelReasonMatchesClaimWinner_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	repo := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithCancelledOutbox(events.BuildCancelledOutboxRow))
	runCancelReasonContract(t, repo, func(t *testing.T, id string) []*orderv1.OrderCancelled {
		r, err := pool.Query(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var payloads [][]byte
		for r.Next() {
			var p []byte
			if err := r.Scan(&p); err != nil {
				t.Fatal(err)
			}
			payloads = append(payloads, p)
		}
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		return decodeCancelPayloads(t, payloads)
	})
}
