package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/migrations"
)

// SA-M8 (durable saga state): a reservation written by one repository instance is
// visible to a freshly-constructed instance over the same backing store — the
// in-memory analogue of surviving a process restart against the same database.
// This proves the saga/reservation state the durable saga relies on is not lost
// when the owning OrderService is torn down and rebuilt.
func TestReservationRepository_SurvivesRestart_SharedStore(t *testing.T) {
	ctx := context.Background()
	shared := store.NewReservationStore()

	// "Process 1" persists a reservation, then goes away.
	repoA := store.NewInMemorySagaRepositoryWithStore(shared)
	if _, err := repoA.CreateReservation(ctx, store.Reservation{
		ID:        "res_durable",
		SagaID:    "saga_1",
		ListingID: "lst_1",
		Quantity:  2,
		Status:    store.ReservationStatusReserved,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create reservation: %v", err)
	}

	// "Process 2" starts fresh over the same durable store and must see it.
	repoB := store.NewInMemorySagaRepositoryWithStore(shared)
	got, err := repoB.GetReservation(ctx, "res_durable")
	if err != nil {
		t.Fatalf("reservation not durable across restart: %v", err)
	}
	if got.ListingID != "lst_1" || got.Quantity != 2 {
		t.Fatalf("reservation data lost across restart: %+v", got)
	}
}

// SA-C2 (durable saga recovery): a Postgres-backed check that a saga + its
// RESERVED reservation written by one PostgresSagaRepository instance are readable
// by a SECOND instance over the same pool — proving the reservation state the
// crash/TTL recovery relies on survives a process restart in the real store. It
// runs only when TEST_DATABASE_URL (or DATABASE_URL) points at a reachable
// Postgres; otherwise it skips, so the unit suite stays green with no database.
func TestSagaReservation_PersistsAcrossRestart_Postgres(t *testing.T) {
	dsn := testDSN(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	applyMigrations(ctx, t, pool)

	// Instance 1 persists a saga header and a RESERVED reservation, then goes away.
	repoA := store.NewPostgresSagaRepository(pool)
	saga, _, err := repoA.CreateSaga(ctx, store.Saga{BuyerID: "buyer_c2"})
	if err != nil {
		t.Fatalf("create saga: %v", err)
	}
	res, err := repoA.CreateReservation(ctx, store.Reservation{
		ID:        "res_pg_durable",
		SagaID:    saga.ID,
		ListingID: "lst_pg",
		Quantity:  4,
		Status:    store.ReservationStatusReserved,
		ExpiresAt: time.Now().Add(-time.Minute), // already expired: the sweep would reclaim it
	})
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}

	// Instance 2 (fresh repo over the same pool) must see the RESERVED reservation,
	// and it must show up in the releasable/sweep set.
	repoB := store.NewPostgresSagaRepository(pool)
	got, err := repoB.GetReservation(ctx, res.ID)
	if err != nil {
		t.Fatalf("reservation not durable across restart: %v", err)
	}
	if got.Status != store.ReservationStatusReserved || got.ListingID != "lst_pg" || got.Quantity != 4 {
		t.Fatalf("reservation state lost across restart: %+v", got)
	}

	releasable, err := repoB.FindReleasable(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("find releasable: %v", err)
	}
	var found bool
	for _, r := range releasable {
		if r.ID == res.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expired RESERVED reservation should be recoverable by the sweep after restart")
	}
}

// SA-M8 (durable returns & shipments): a Postgres-backed integration check that
// returns/shipments written by one repo instance are readable by a new instance
// over the same pool. It runs only when TEST_DATABASE_URL (or DATABASE_URL) points
// at a reachable Postgres with the migrations applied; otherwise it skips, so the
// unit suite stays green where no database is provisioned.
func TestReturnsAndShipments_PersistAcrossRestart_Postgres(t *testing.T) {
	dsn := testDSN(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	applyMigrations(ctx, t, pool)

	// Seed an order to satisfy the FK on returns/shipments.
	orderRepo := store.NewPostgresOrderRepository(pool)
	order, err := orderRepo.CreateOrder(ctx, store.Order{
		BuyerID: "buyer_m8", SellerID: "seller_m8", Status: store.OrderStatusCompleted,
		TotalAmount: 100000, Currency: "VND",
	})
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}

	// Instance 1 writes a return and a shipment.
	retRepoA := store.NewPostgresReturnRepository(pool)
	ret, err := retRepoA.CreateReturn(ctx, store.OrderReturn{
		OrderID: order.ID, BuyerID: "buyer_m8", SellerID: "seller_m8",
		Reason: "durability check", RefundAmount: 100000,
	})
	if err != nil {
		t.Fatalf("create return: %v", err)
	}
	shipRepoA := store.NewPostgresShipmentRepository(pool)
	ship, err := shipRepoA.CreateShipment(ctx, store.Shipment{
		OrderID: order.ID, Carrier: "GHN", TrackingCode: "M8-" + order.ID,
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}

	// Instance 2 (fresh repos over the same pool) must read them back.
	if got, err := store.NewPostgresReturnRepository(pool).GetReturn(ctx, ret.ID); err != nil || got.ID != ret.ID {
		t.Fatalf("return not durable across restart: got %+v err %v", got, err)
	}
	if got, err := store.NewPostgresShipmentRepository(pool).GetShipment(ctx, ship.ID); err != nil || got.ID != ship.ID {
		t.Fatalf("shipment not durable across restart: got %+v err %v", got, err)
	}
}

// applyMigrations applies every migration to the database pool is connected to, the
// way the deploy migrate job does.
func applyMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if err := migrate.Up(ctx, pool.Config().ConnString(), migrations.Files); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}
