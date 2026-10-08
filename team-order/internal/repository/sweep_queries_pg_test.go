package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
)

func TestSweepQueries_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	orders := repository.NewPostgresOrderRepository(pool)
	sagas := repository.NewPostgresSagaRepository(pool)

	sagaID, res := newCheckout(t, sagas, "la", "lb")
	live, cancelled := uid("ord"), uid("ord")
	if _, err := orders.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{
		{Order: order(live, "sa"), ReservationIDs: res[:1]},
		{Order: order(cancelled, "sb"), ReservationIDs: res[1:]},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 5 WHERE id = $1`, cancelled); err != nil {
		t.Fatal(err)
	}

	byOrder, err := sagas.ListReservationsByOrder(ctx, cancelled)
	if err != nil || len(byOrder) != 1 || byOrder[0].ID != res[1] {
		t.Fatalf("ListReservationsByOrder: %v %+v", err, byOrder)
	}

	held, err := sagas.FindHeldByCancelledOrders(ctx, time.Now().Add(time.Second), 1000)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, r := range held {
		found[r.ID] = true
	}
	if !found[res[1]] || found[res[0]] {
		t.Fatalf("want only the cancelled order's reservation, got %v", found)
	}
	if early, _ := sagas.FindHeldByCancelledOrders(ctx, time.Now().Add(-time.Hour), 1000); containsRes(early, res[1]) {
		t.Fatal("an order cancelled after the cut-off must not be returned")
	}

	pending, _, _ := sagas.CreateSaga(ctx, repository.Saga{BuyerID: uid("b")})
	stale, err := sagas.FindStalePendingSagas(ctx, time.Now().Add(time.Second), 100000)
	if err != nil {
		t.Fatal(err)
	}
	var sawPending, sawCompleted bool
	for _, s := range stale {
		sawPending = sawPending || s.ID == pending.ID
		sawCompleted = sawCompleted || s.ID == sagaID
	}
	if !sawPending || sawCompleted {
		t.Fatalf("stale pending sagas: pending=%v completed=%v", sawPending, sawCompleted)
	}
}

func containsRes(rs []repository.Reservation, id string) bool {
	for _, r := range rs {
		if r.ID == id {
			return true
		}
	}
	return false
}
