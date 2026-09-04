package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

// newTestPool connects to a real Postgres for the repository round-trip tests.
// It skips (never fails) when no database is reachable — the container running
// `go test` has none, so these stay green while still exercising the pgx paths
// wherever a DB is provided via PROMOTION_TEST_DATABASE_URL (or DATABASE_URL).
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PROMOTION_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("no PROMOTION_TEST_DATABASE_URL/DATABASE_URL set; skipping Postgres repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("connect Postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("ping Postgres: %v", err)
	}

	schema, err := os.ReadFile("../../migrations/0001_promotion.up.sql")
	if err != nil {
		pool.Close()
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		pool.Close()
		t.Fatalf("apply migration: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		// Clean up rows this test wrote; leave the schema in place (IF NOT EXISTS).
		_, _ = pool.Exec(cctx, `TRUNCATE flash_sale_campaigns, voucher_reservations, vouchers CASCADE`)
		pool.Close()
	})
	return pool
}

func TestPostgresRepositories(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	vrepo := repository.NewPostgresVoucherRepository(pool)
	rrepo := repository.NewPostgresReservationRepository(pool)
	frepo := repository.NewPostgresFlashSaleRepository(pool)

	// Voucher round-trip + used increment.
	v, err := vrepo.Create(ctx, repository.Voucher{
		Code: "PG10", Scope: 2, DiscountType: 1, DiscountValue: 10, Quota: 5,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("voucher create: %v", err)
	}
	if got, err := vrepo.GetByCode(ctx, "PG10"); err != nil || got.ID != v.ID {
		t.Fatalf("GetByCode: got=%+v err=%v", got, err)
	}
	if err := vrepo.IncrementUsed(ctx, v.ID, 1); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}

	// Reservation idempotency via UNIQUE(reservation_id).
	_, created, err := rrepo.Create(ctx, repository.Reservation{
		ReservationID: "pg-resv", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 500,
	})
	if err != nil || !created {
		t.Fatalf("first reservation: created=%v err=%v", created, err)
	}
	_, created, err = rrepo.Create(ctx, repository.Reservation{
		ReservationID: "pg-resv", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 999,
	})
	if err != nil {
		t.Fatalf("second reservation: %v", err)
	}
	if created {
		t.Fatal("second reservation must be idempotent (created=false)")
	}
	got, err := rrepo.GetByReservationID(ctx, "pg-resv")
	if err != nil || got.DiscountAmount != 500 {
		t.Fatalf("reservation kept wrong discount: %+v err=%v", got, err)
	}

	// Flash-sale active window.
	c, err := frepo.Create(ctx, repository.FlashSaleCampaign{
		ListingID: "pg-listing", SalePrice: 100, StockCap: 50, StockSold: 5,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	activeC, err := frepo.GetActiveByListing(ctx, "pg-listing", now)
	if err != nil || activeC.ID != c.ID {
		t.Fatalf("GetActiveByListing: got=%+v err=%v", activeC, err)
	}
}
