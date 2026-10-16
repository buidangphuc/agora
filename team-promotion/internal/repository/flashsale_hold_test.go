package repository_test

// Tests for the flash-sale hold repository contract (openspec change
// promotion-flash-stock-hold). The same contract runs against the in-memory
// backend and, when a database is configured, against Postgres.
//
// Interface assumed by these tests (the implementer creates it in
// internal/repository/flashsale_hold.go):
//
//	const FlashHoldTTL = 15 * time.Minute
//	const FlashHoldReserved, FlashHoldCommitted, FlashHoldReleased = "reserved", "committed", "released"
//	var ErrFlashHoldNotFound, ErrFlashStockInsufficient error
//	type FlashHold struct { ReservationID, CampaignID, ListingID, VariantID string
//	    Quantity int64; Status string; ExpiresAt time.Time }
//	type FlashHoldRepository interface {
//	    // Reserve creates the hold (ExpiresAt = now+FlashHoldTTL) under the campaign
//	    // lock, or returns the stored hold with created=false for a known
//	    // reservation id. ErrCampaignNotFound / ErrFlashStockInsufficient on refusal.
//	    Reserve(ctx, h FlashHold, now time.Time) (stored FlashHold, created bool, err error)
//	    Get(ctx, reservationID string) (FlashHold, error)            // raw row; ErrFlashHoldNotFound
//	    Commit(ctx, reservationID string, now time.Time) (bool, error)
//	    Release(ctx, reservationID string, now time.Time) (found bool, err error)
//	    ActiveHeld(ctx, campaignID string, now time.Time) (int64, error)
//	    PurgeReleasedAndExpiredHolds(ctx, before time.Time) (int64, error)
//	}
//	func NewInMemoryFlashHoldRepository(*InMemoryFlashSaleRepository) *InMemoryFlashHoldRepository
//	func NewPostgresFlashHoldRepository(*pgxpool.Pool) *PostgresFlashHoldRepository
//
// Time is always passed in, so no test depends on the wall clock.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

var holdT0 = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

type flashHoldEnv struct {
	campaigns repository.FlashSaleRepository
	holds     repository.FlashHoldRepository
}

// forEachHoldBackend runs fn against the in-memory backend and Postgres
// (the latter skips without a database).
func forEachHoldBackend(t *testing.T, fn func(t *testing.T, env flashHoldEnv)) {
	t.Helper()
	t.Run("inmemory", func(t *testing.T) {
		c := repository.NewInMemoryFlashSaleRepository()
		fn(t, flashHoldEnv{campaigns: c, holds: repository.NewInMemoryFlashHoldRepository(c)})
	})
	t.Run("postgres", func(t *testing.T) {
		pool := newHoldTestPool(t)
		fn(t, flashHoldEnv{
			campaigns: repository.NewPostgresFlashSaleRepository(pool),
			holds:     repository.NewPostgresFlashHoldRepository(pool),
		})
	})
}

// newHoldTestPool is newTestPool plus migration 0004.
func newHoldTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newTestPool(t)
	up, err := os.ReadFile("../../migrations/0004_flash_sale_holds.up.sql")
	if err != nil {
		t.Fatalf("read 0004 up: %v", err)
	}
	if _, err := pool.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("apply 0004 up: %v", err)
	}
	return pool
}

func (e flashHoldEnv) campaign(t *testing.T, stockCap, stockSold int64) repository.FlashSaleCampaign {
	t.Helper()
	c, err := e.campaigns.Create(context.Background(), repository.FlashSaleCampaign{
		ListingID: "l1", SalePrice: 90000, StockCap: stockCap, StockSold: stockSold,
		StartsAt: holdT0.Add(-24 * time.Hour), EndsAt: holdT0.Add(24 * time.Hour * 365),
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return c
}

func (e flashHoldEnv) reserve(t *testing.T, c repository.FlashSaleCampaign, id string, qty int64, now time.Time) error {
	t.Helper()
	_, _, err := e.holds.Reserve(context.Background(), repository.FlashHold{
		ReservationID: c.ID + ":" + id, CampaignID: c.ID, ListingID: c.ListingID, Quantity: qty,
	}, now)
	return err
}

func (e flashHoldEnv) rid(c repository.FlashSaleCampaign, id string) string { return c.ID + ":" + id }

func (e flashHoldEnv) held(t *testing.T, c repository.FlashSaleCampaign, now time.Time) int64 {
	t.Helper()
	n, err := e.holds.ActiveHeld(context.Background(), c.ID, now)
	if err != nil {
		t.Fatalf("ActiveHeld: %v", err)
	}
	return n
}

func (e flashHoldEnv) sold(t *testing.T, c repository.FlashSaleCampaign) int64 {
	t.Helper()
	got, err := e.campaigns.GetByID(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return got.StockSold
}

func (e flashHoldEnv) exists(t *testing.T, c repository.FlashSaleCampaign, id string) bool {
	t.Helper()
	_, err := e.holds.Get(context.Background(), e.rid(c, id))
	if err == nil {
		return true
	}
	if errors.Is(err, repository.ErrFlashHoldNotFound) {
		return false
	}
	t.Fatalf("Get: %v", err)
	return false
}

// Scenario: A hold expires 15 minutes after reserve.
func TestFlashHoldRepoExpiresAfterFifteenMinutes(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)
		stored, created, err := e.holds.Reserve(ctx, repository.FlashHold{
			ReservationID: e.rid(c, "r1"), CampaignID: c.ID, ListingID: "l1", Quantity: 2,
		}, holdT0)
		if err != nil || !created {
			t.Fatalf("reserve: created=%v err=%v", created, err)
		}
		if !stored.ExpiresAt.Equal(holdT0.Add(15 * time.Minute)) {
			t.Fatalf("ExpiresAt = %v, want %v", stored.ExpiresAt, holdT0.Add(15*time.Minute))
		}
		if got := e.held(t, c, holdT0.Add(14*time.Minute+59*time.Second)); got != 2 {
			t.Fatalf("held at 14:59 = %d, want 2", got)
		}
		if got := e.held(t, c, holdT0.Add(15*time.Minute)); got != 0 {
			t.Fatalf("held at 15:00 = %d, want 0", got)
		}
		if err := e.reserve(t, c, "r2", 5, holdT0.Add(15*time.Minute)); err != nil {
			t.Fatalf("reserve of the full cap after expiry: %v", err)
		}
	})
}

// Scenario: Reserve refuses more units than remain / accepts exactly the remaining units.
func TestFlashHoldRepoCapBoundary(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		c := e.campaign(t, 5, 2) // 3 remain
		if err := e.reserve(t, c, "a", 1, holdT0); err != nil {
			t.Fatalf("reserve 1: %v", err)
		}
		if err := e.reserve(t, c, "b", 3, holdT0); !errors.Is(err, repository.ErrFlashStockInsufficient) {
			t.Fatalf("reserve 3 of 2 remaining: err=%v, want ErrFlashStockInsufficient", err)
		}
		if e.exists(t, c, "b") {
			t.Fatal("refused reserve left a hold")
		}
		if err := e.reserve(t, c, "c", 2, holdT0); err != nil {
			t.Fatalf("reserve exactly the remaining 2: %v", err)
		}
		if got := e.held(t, c, holdT0); got != 3 {
			t.Fatalf("held = %d, want 3", got)
		}
	})
}

// Scenario: Reserve refuses an unknown campaign.
func TestFlashHoldRepoUnknownCampaign(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		_, created, err := e.holds.Reserve(context.Background(), repository.FlashHold{
			ReservationID: "orphan-" + fmt.Sprint(time.Now().UnixNano()), CampaignID: "nope", ListingID: "l1", Quantity: 1,
		}, holdT0)
		if !errors.Is(err, repository.ErrCampaignNotFound) || created {
			t.Fatalf("created=%v err=%v, want ErrCampaignNotFound", created, err)
		}
	})
}

// Scenario: Reserve is idempotent on the reservation id / replay with different fields.
func TestFlashHoldRepoReserveIdempotent(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)
		h := repository.FlashHold{ReservationID: e.rid(c, "r1"), CampaignID: c.ID, ListingID: "l1", Quantity: 2}
		if _, created, err := e.holds.Reserve(ctx, h, holdT0); err != nil || !created {
			t.Fatalf("first: created=%v err=%v", created, err)
		}
		h.Quantity = 4
		stored, created, err := e.holds.Reserve(ctx, h, holdT0.Add(time.Minute))
		if err != nil || created {
			t.Fatalf("replay: created=%v err=%v, want stored hold, created=false", created, err)
		}
		if stored.Quantity != 2 || stored.Status != repository.FlashHoldReserved {
			t.Fatalf("replay stored = %+v, want quantity 2 status reserved", stored)
		}
		if got := e.held(t, c, holdT0.Add(time.Minute)); got != 2 {
			t.Fatalf("held = %d, want 2 (hold once)", got)
		}
	})
}

// Scenario: Concurrent reserves of one reservation id hold once.
func TestFlashHoldRepoConcurrentSameReservation(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)
		var wg sync.WaitGroup
		var mu sync.Mutex
		created, failed := 0, 0
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, cr, err := e.holds.Reserve(ctx, repository.FlashHold{
					ReservationID: e.rid(c, "same"), CampaignID: c.ID, ListingID: "l1", Quantity: 2,
				}, holdT0)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
				} else if cr {
					created++
				}
			}()
		}
		wg.Wait()
		if failed != 0 || created != 1 {
			t.Fatalf("created=%d failed=%d, want exactly 1 created and 0 errors", created, failed)
		}
		if got := e.held(t, c, holdT0); got != 2 {
			t.Fatalf("held = %d, want 2", got)
		}
	})
}

// Scenario: Concurrent reserves are capped.
func TestFlashHoldRepoConcurrentCap(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) { runConcurrentCap(t, e) })
}

// Scenario: Concurrent reserves are capped (Postgres, FOR UPDATE on the campaign row).
func TestPostgresFlashHoldConcurrentCap(t *testing.T) {
	pool := newHoldTestPool(t)
	runConcurrentCap(t, flashHoldEnv{
		campaigns: repository.NewPostgresFlashSaleRepository(pool),
		holds:     repository.NewPostgresFlashHoldRepository(pool),
	})
}

func runConcurrentCap(t *testing.T, e flashHoldEnv) {
	t.Helper()
	c := e.campaign(t, 5, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, insufficient, other := 0, 0, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := e.reserve(t, c, fmt.Sprintf("r%d", i), 1, holdT0)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, repository.ErrFlashStockInsufficient):
				insufficient++
			default:
				other++
			}
		}(i)
	}
	wg.Wait()
	if ok != 5 || insufficient != 15 || other != 0 {
		t.Fatalf("ok=%d insufficient=%d other=%d, want 5/15/0", ok, insufficient, other)
	}
	if got := e.held(t, c, holdT0); got != 5 {
		t.Fatalf("held = %d, want 5", got)
	}
}

// Scenario: Commit adds the quantity to stock_sold / is idempotent / refused cases.
func TestFlashHoldRepoCommit(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)
		if err := e.reserve(t, c, "r1", 2, holdT0); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ { // second commit is a no-op that still reports true
			if ok, err := e.holds.Commit(ctx, e.rid(c, "r1"), holdT0.Add(time.Minute)); err != nil || !ok {
				t.Fatalf("commit #%d: ok=%v err=%v", i+1, ok, err)
			}
			if got := e.sold(t, c); got != 2 {
				t.Fatalf("stock_sold after commit #%d = %d, want 2", i+1, got)
			}
		}
		// A committed hold never expires and no longer counts as "held".
		if got := e.held(t, c, holdT0.Add(time.Minute)); got != 0 {
			t.Fatalf("held after commit = %d, want 0 (units are in stock_sold)", got)
		}
		if ok, err := e.holds.Commit(ctx, e.rid(c, "ghost"), holdT0); err != nil || ok {
			t.Fatalf("unknown commit: ok=%v err=%v, want false,nil", ok, err)
		}
		// Expired hold: refused, no change.
		if err := e.reserve(t, c, "r2", 1, holdT0); err != nil {
			t.Fatal(err)
		}
		if ok, err := e.holds.Commit(ctx, e.rid(c, "r2"), holdT0.Add(15*time.Minute)); err != nil || ok {
			t.Fatalf("expired commit: ok=%v err=%v, want false,nil", ok, err)
		}
		// Released hold: refused, no change.
		if err := e.reserve(t, c, "r3", 1, holdT0); err != nil {
			t.Fatal(err)
		}
		if _, err := e.holds.Release(ctx, e.rid(c, "r3"), holdT0); err != nil {
			t.Fatal(err)
		}
		if ok, err := e.holds.Commit(ctx, e.rid(c, "r3"), holdT0); err != nil || ok {
			t.Fatalf("released commit: ok=%v err=%v, want false,nil", ok, err)
		}
		if got := e.sold(t, c); got != 2 {
			t.Fatalf("stock_sold = %d, want 2 (refused commits change nothing)", got)
		}
	})
}

// Scenario: Release returns the units of an active/committed hold; idempotent; unknown; expired.
func TestFlashHoldRepoRelease(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)

		// Active hold.
		if err := e.reserve(t, c, "active", 2, holdT0); err != nil {
			t.Fatal(err)
		}
		if found, err := e.holds.Release(ctx, e.rid(c, "active"), holdT0); err != nil || !found {
			t.Fatalf("release active: found=%v err=%v", found, err)
		}
		if got := e.held(t, c, holdT0); got != 0 {
			t.Fatalf("held after release = %d, want 0", got)
		}
		if h, err := e.holds.Get(ctx, e.rid(c, "active")); err != nil || h.Status != repository.FlashHoldReleased {
			t.Fatalf("status after release = %+v err=%v, want released", h, err)
		}

		// Committed hold: release decrements stock_sold exactly once.
		if err := e.reserve(t, c, "sold", 2, holdT0); err != nil {
			t.Fatal(err)
		}
		if ok, err := e.holds.Commit(ctx, e.rid(c, "sold"), holdT0); err != nil || !ok {
			t.Fatalf("commit: ok=%v err=%v", ok, err)
		}
		for i := 0; i < 2; i++ {
			if found, err := e.holds.Release(ctx, e.rid(c, "sold"), holdT0.Add(time.Hour)); err != nil || !found {
				t.Fatalf("release committed #%d: found=%v err=%v", i+1, found, err)
			}
			if got := e.sold(t, c); got != 0 {
				t.Fatalf("stock_sold after release #%d = %d, want 0", i+1, got)
			}
		}

		// Unknown: found=false, no error.
		if found, err := e.holds.Release(ctx, e.rid(c, "ghost"), holdT0); err != nil || found {
			t.Fatalf("release unknown: found=%v err=%v, want false,nil", found, err)
		}

		// Expired uncommitted hold: found=true, no counter change.
		if err := e.reserve(t, c, "late", 1, holdT0); err != nil {
			t.Fatal(err)
		}
		if found, err := e.holds.Release(ctx, e.rid(c, "late"), holdT0.Add(15*time.Minute)); err != nil || !found {
			t.Fatalf("release expired: found=%v err=%v, want true,nil", found, err)
		}
		if got := e.sold(t, c); got != 0 {
			t.Fatalf("stock_sold after releasing an expired hold = %d, want 0", got)
		}
	})
}

// Scenario: Released and expired holds are purged after 30 days.
func TestPurgeReleasedAndExpiredHolds(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 100, 0)
		day := 24 * time.Hour
		now := holdT0

		// Released 31 days ago.
		must(t, e.reserve(t, c, "released31", 1, now.Add(-31*day)))
		if _, err := e.holds.Release(ctx, e.rid(c, "released31"), now.Add(-31*day)); err != nil {
			t.Fatal(err)
		}
		// Expired uncommitted, expiry 31 days ago.
		must(t, e.reserve(t, c, "expired31", 1, now.Add(-31*day-15*time.Minute)))
		// Released 29 days ago.
		must(t, e.reserve(t, c, "released29", 1, now.Add(-29*day)))
		if _, err := e.holds.Release(ctx, e.rid(c, "released29"), now.Add(-29*day)); err != nil {
			t.Fatal(err)
		}
		// Active hold.
		must(t, e.reserve(t, c, "active", 1, now))

		if _, err := e.holds.PurgeReleasedAndExpiredHolds(ctx, now.Add(-30*day)); err != nil {
			t.Fatalf("purge: %v", err)
		}
		for id, want := range map[string]bool{"released31": false, "expired31": false, "released29": true, "active": true} {
			if got := e.exists(t, c, id); got != want {
				t.Errorf("hold %s exists=%v, want %v", id, got, want)
			}
		}
	})
}

// Scenario: A committed hold older than 30 days is not purged.
func TestPurgeKeepsCommittedHolds(t *testing.T) {
	forEachHoldBackend(t, func(t *testing.T, e flashHoldEnv) {
		ctx := context.Background()
		c := e.campaign(t, 5, 0)
		day := 24 * time.Hour
		old := holdT0.Add(-90 * day)

		must(t, e.reserve(t, c, "old", 2, old))
		if ok, err := e.holds.Commit(ctx, e.rid(c, "old"), old); err != nil || !ok {
			t.Fatalf("commit: ok=%v err=%v", ok, err)
		}
		if _, err := e.holds.PurgeReleasedAndExpiredHolds(ctx, holdT0.Add(-30*day)); err != nil {
			t.Fatalf("purge: %v", err)
		}
		if !e.exists(t, c, "old") {
			t.Fatal("committed 90-day-old hold was purged")
		}
		if found, err := e.holds.Release(ctx, e.rid(c, "old"), holdT0); err != nil || !found {
			t.Fatalf("release after purge: found=%v err=%v, want true,nil", found, err)
		}
		if got := e.sold(t, c); got != 0 {
			t.Fatalf("stock_sold = %d, want 0", got)
		}
	})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Scenario: Migration applies and rolls back.
func TestPostgresFlashHoldMigrationRoundTrip(t *testing.T) {
	pool := newTestPool(t) // migration 0001 only; the 0004 steps below are explicit
	ctx := context.Background()
	up, err := os.ReadFile("../../migrations/0004_flash_sale_holds.up.sql")
	if err != nil {
		t.Fatalf("read 0004 up: %v", err)
	}
	down, err := os.ReadFile("../../migrations/0004_flash_sale_holds.down.sql")
	if err != nil {
		t.Fatalf("read 0004 down: %v", err)
	}
	exists := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('flash_sale_holds') IS NOT NULL`).Scan(&ok); err != nil {
			t.Fatalf("to_regclass: %v", err)
		}
		return ok
	}
	apply := func(name string, sql []byte, wantTable bool) {
		t.Helper()
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := exists(); got != wantTable {
			t.Fatalf("after %s: table exists=%v, want %v", name, got, wantTable)
		}
	}

	// A pre-existing campaign row must survive the whole cycle untouched.
	camp, err := repository.NewPostgresFlashSaleRepository(pool).Create(ctx, repository.FlashSaleCampaign{
		ListingID: "l-migration", SalePrice: 1, StockCap: 7, StockSold: 3,
	})
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	apply("up", up, true)
	apply("down", down, false)
	apply("up again", up, true)
	apply("up a third time (IF NOT EXISTS)", up, true)

	for _, col := range []string{"reservation_id", "campaign_id", "listing_id", "variant_id", "quantity", "status", "expires_at"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.columns WHERE table_name='flash_sale_holds' AND column_name=$1`, col).Scan(&n); err != nil || n != 1 {
			t.Errorf("column %s: count=%d err=%v, want 1", col, n, err)
		}
	}
	got, err := repository.NewPostgresFlashSaleRepository(pool).GetByID(ctx, camp.ID)
	if err != nil || got.StockSold != 3 || got.StockCap != 7 {
		t.Fatalf("campaign after migration cycle = %+v err=%v, want untouched", got, err)
	}
}
