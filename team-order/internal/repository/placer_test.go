package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-order/internal/repository"
)

type placerFixture struct {
	placer  repository.OrderPlacer
	orders  repository.OrderRepository
	sagas   repository.SagaRepository
	countFn func(orderIDs ...string) int
}

// newCheckout opens a saga with one RESERVED reservation per listing and returns
// the saga id and the reservation ids.
func newCheckout(t *testing.T, sagas repository.SagaRepository, listings ...string) (string, []string) {
	t.Helper()
	ctx := context.Background()
	sg, err := sagas.CreateSaga(ctx, repository.Saga{BuyerID: "buyer_p"})
	if err != nil {
		t.Fatalf("create saga: %v", err)
	}
	var ids []string
	for _, l := range listings {
		id := uid("res")
		if _, err := sagas.CreateReservation(ctx, repository.Reservation{
			ID: id, SagaID: sg.ID, BuyerID: "buyer_p", ListingID: l, Quantity: 1,
			Status: repository.ReservationStatusReserved, ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("create reservation: %v", err)
		}
		ids = append(ids, id)
	}
	return sg.ID, ids
}

func order(id, seller string) repository.Order {
	return repository.Order{ID: id, BuyerID: "buyer_p", SellerID: seller, TotalAmount: 10,
		Items: []repository.OrderItem{{ListingID: "l-" + seller, Quantity: 1, UnitPrice: 10}}}
}

func runPlacerContract(t *testing.T, fx func(t *testing.T) placerFixture) {
	ctx := context.Background()

	t.Run("everything commits together", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la", "lb")
		oa, ob := uid("ord"), uid("ord")
		got, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{
			{Order: order(oa, "sa"), ReservationIDs: res[:1]},
			{Order: order(ob, "sb"), ReservationIDs: res[1:]},
		})
		if err != nil || len(got) != 2 {
			t.Fatalf("place: %v %d", err, len(got))
		}
		if n := f.countFn(oa, ob); n != 2 {
			t.Fatalf("want 2 orders, got %d", n)
		}
		for i, id := range res {
			r, _ := f.sagas.GetReservation(ctx, id)
			want := []string{oa, ob}[i]
			if r.Status != repository.ReservationStatusCommitted || r.OrderID != want {
				t.Fatalf("reservation %s: %+v, want COMMITTED to %s", id, r, want)
			}
		}
		all, _ := f.sagas.ListReservationsBySaga(ctx, sagaID)
		if len(all) != 2 {
			t.Fatalf("want 2 reservations, got %d", len(all))
		}
		if o, err := f.orders.GetOrder(ctx, oa); err != nil || len(o.Items) != 1 || o.Status != repository.OrderStatusPending {
			t.Fatalf("order a: %v %+v", err, o)
		}
	})

	t.Run("a failed binding leaves zero orders", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la")
		oa, ob := uid("ord"), uid("ord")
		_, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{
			{Order: order(oa, "sa"), ReservationIDs: res},
			{Order: order(ob, "sb"), ReservationIDs: []string{uid("missing")}},
		})
		if !errors.Is(err, repository.ErrReservationLost) {
			t.Fatalf("want ErrReservationLost, got %v", err)
		}
		if n := f.countFn(oa, ob); n != 0 {
			t.Fatalf("want 0 orders after a failed binding, got %d", n)
		}
		if r, _ := f.sagas.GetReservation(ctx, res[0]); r.Status != repository.ReservationStatusReserved || r.OrderID != "" {
			t.Fatalf("first binding must roll back: %+v", r)
		}
	})

	t.Run("a released reservation rolls everything back", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la", "lb")
		if err := f.sagas.UpdateReservationStatus(ctx, res[1], repository.ReservationStatusReleased); err != nil {
			t.Fatal(err)
		}
		oa := uid("ord")
		_, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{{Order: order(oa, "sa"), ReservationIDs: res}})
		if !errors.Is(err, repository.ErrReservationLost) {
			t.Fatalf("want ErrReservationLost, got %v", err)
		}
		if n := f.countFn(oa); n != 0 {
			t.Fatalf("want 0 orders, got %d", n)
		}
	})

	t.Run("a saga no longer pending refuses placement", func(t *testing.T) {
		f := fx(t)
		sagaID, res := newCheckout(t, f.sagas, "la")
		if err := f.sagas.UpdateSagaStatus(ctx, sagaID, repository.SagaStatusCompensated); err != nil {
			t.Fatal(err)
		}
		oa := uid("ord")
		if _, err := f.placer.PlaceOrders(ctx, sagaID, []repository.PlacedOrder{{Order: order(oa, "sa"), ReservationIDs: res}}); !errors.Is(err, repository.ErrReservationLost) {
			t.Fatalf("want ErrReservationLost, got %v", err)
		}
		if n := f.countFn(oa); n != 0 {
			t.Fatalf("want 0 orders, got %d", n)
		}
	})
}

func TestOrderPlacer_InMemory(t *testing.T) {
	runPlacerContract(t, func(t *testing.T) placerFixture {
		orders := repository.NewInMemoryOrderRepository()
		sagas := repository.NewInMemorySagaRepository()
		return placerFixture{
			placer: repository.NewInMemoryOrderPlacer(orders, sagas), orders: orders, sagas: sagas,
			countFn: func(ids ...string) int {
				n := 0
				for _, id := range ids {
					if _, err := orders.GetOrder(context.Background(), id); err == nil {
						n++
					}
				}
				return n
			},
		}
	})
}

func TestOrderPlacer_Postgres(t *testing.T) {
	pool := pgPool(t)
	runPlacerContract(t, func(t *testing.T) placerFixture {
		orders := repository.NewPostgresOrderRepository(pool)
		return placerFixture{
			placer: orders, orders: orders, sagas: repository.NewPostgresSagaRepository(pool),
			countFn: func(ids ...string) int { return countOrders(t, pool, ids...) },
		}
	})
}

func countOrders(t *testing.T, pool *pgxpool.Pool, ids ...string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE id = ANY($1)`, ids).Scan(&n); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	var items int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM order_items WHERE order_id = ANY($1)`, ids).Scan(&items); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if n == 0 && items != 0 {
		t.Fatalf("orphan order items: %d", items)
	}
	return n
}
