package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
)

var farFuture = time.Now().Add(time.Hour)

func TestPG_CommitReservation_Outcomes(t *testing.T) {
	repo, _ := newPGRepo(t)
	ctx := context.Background()
	seedPGListing(t, repo, "L1", 10)

	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, farFuture); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := repo.CommitReservation(ctx, "r1"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := repo.CommitReservation(ctx, "r1"); err != nil {
		t.Fatalf("idempotent commit: %v", err)
	}
	if err := repo.CommitReservation(ctx, "nope"); !errors.Is(err, repository.ErrReservationNotFound) {
		t.Fatalf("unknown: want ErrReservationNotFound, got %v", err)
	}

	// released: reserve with an already-past TTL, sweep, then commit.
	if err := repo.ReserveStockIdempotent(ctx, "r2", "L1", "", 3, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("reserve r2: %v", err)
	}
	if n, err := repo.SweepExpiredReservations(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v, want 1/nil", n, err)
	}
	if err := repo.CommitReservation(ctx, "r2"); !errors.Is(err, repository.ErrReservationReleased) {
		t.Fatalf("released: want ErrReservationReleased, got %v", err)
	}
	if got := pgStock(t, repo, "L1"); got != 8 {
		t.Errorf("stock = %d, want 8 (r1 kept, r2 restored)", got)
	}
}

// A committed reservation is never restored by the TTL sweep, even long after
// its expires_at (a placed order keeps its stock).
func TestPG_CommittedReservationSurvivesSweep(t *testing.T) {
	repo, _ := newPGRepo(t)
	ctx := context.Background()
	seedPGListing(t, repo, "L1", 10)

	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := repo.CommitReservation(ctx, "r1"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n, err := repo.SweepExpiredReservations(ctx, time.Now().Add(24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("sweep n=%d err=%v, want 0/nil", n, err)
	}
	if got := pgStock(t, repo, "L1"); got != 8 {
		t.Fatalf("stock = %d, want 8 (committed reservation kept)", got)
	}
}

// Commit racing the sweeper on the same expired reservation: exactly one wins and
// the stock matches the winner.
func TestPG_CommitVersusSweepConcurrent(t *testing.T) {
	repo, _ := newPGRepo(t)
	ctx := context.Background()
	seedPGListing(t, repo, "L1", 1000)

	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("race-%d", i)
		before := pgStock(t, repo, "L1")
		if err := repo.ReserveStockIdempotent(ctx, id, "L1", "", 4, time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		var wg sync.WaitGroup
		var commitErr error
		wg.Add(2)
		go func() { defer wg.Done(); commitErr = repo.CommitReservation(ctx, id) }()
		go func() { defer wg.Done(); _, _ = repo.SweepExpiredReservations(ctx, time.Now()) }()
		wg.Wait()

		got := pgStock(t, repo, "L1")
		switch {
		case commitErr == nil:
			if got != before-4 {
				t.Fatalf("iter %d: commit won, stock=%d want %d", i, got, before-4)
			}
		case errors.Is(commitErr, repository.ErrReservationReleased):
			if got != before {
				t.Fatalf("iter %d: sweep won, stock=%d want %d", i, got, before)
			}
		default:
			t.Fatalf("iter %d: commit error %v", i, commitErr)
		}
	}
}

func TestPG_ReleaseReservation(t *testing.T) {
	ctx := context.Background()

	t.Run("double release restores once", func(t *testing.T) {
		repo, _ := newPGRepo(t)
		seedPGListing(t, repo, "L1", 10)
		if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 3, farFuture); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if out, err := repo.ReleaseReservation(ctx, "r1"); err != nil || out != repository.ReleaseApplied {
			t.Fatalf("first release: out=%v err=%v", out, err)
		}
		if out, err := repo.ReleaseReservation(ctx, "r1"); err != nil || out != repository.ReleaseNoOp {
			t.Fatalf("second release: out=%v err=%v, want NoOp", out, err)
		}
		if got := pgStock(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("release after sweep is a no-op", func(t *testing.T) {
		repo, _ := newPGRepo(t)
		seedPGListing(t, repo, "L1", 10)
		if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 3, time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if n, err := repo.SweepExpiredReservations(ctx, time.Now()); err != nil || n != 1 {
			t.Fatalf("sweep n=%d err=%v", n, err)
		}
		if out, err := repo.ReleaseReservation(ctx, "r1"); err != nil || out != repository.ReleaseNoOp {
			t.Fatalf("release: out=%v err=%v, want NoOp", out, err)
		}
		if got := pgStock(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("unknown id is a no-op", func(t *testing.T) {
		repo, _ := newPGRepo(t)
		seedPGListing(t, repo, "L1", 10)
		if out, err := repo.ReleaseReservation(ctx, "never"); err != nil || out != repository.ReleaseUnknown {
			t.Fatalf("release: out=%v err=%v, want Unknown", out, err)
		}
		if got := pgStock(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("stored quantity is restored, variant too", func(t *testing.T) {
		repo, pool := newPGRepo(t)
		if _, err := repo.Create(ctx, repository.Listing{
			ID: "L1", Title: "L1", Currency: "VND", Status: "published", Stock: 0,
			Variants: []repository.Variant{{ID: "V1", Name: "red", SKU: "r", Stock: 6}},
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "V1", 4, farFuture); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if _, err := repo.ReleaseReservation(ctx, "r1"); err != nil {
			t.Fatalf("release: %v", err)
		}
		var st int32
		if err := pool.QueryRow(ctx, `SELECT stock FROM listing_variants WHERE id = 'V1'`).Scan(&st); err != nil || st != 6 {
			t.Fatalf("variant stock = %d err=%v, want 6", st, err)
		}
	})

	t.Run("committed is releasable", func(t *testing.T) {
		repo, _ := newPGRepo(t)
		seedPGListing(t, repo, "L1", 10)
		if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, farFuture); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if err := repo.CommitReservation(ctx, "r1"); err != nil {
			t.Fatalf("commit: %v", err)
		}
		if out, err := repo.ReleaseReservation(ctx, "r1"); err != nil || out != repository.ReleaseApplied {
			t.Fatalf("release: out=%v err=%v", out, err)
		}
		if got := pgStock(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
		if err := repo.CommitReservation(ctx, "r1"); !errors.Is(err, repository.ErrReservationReleased) {
			t.Fatalf("commit after release: %v, want ErrReservationReleased", err)
		}
	})

	t.Run("concurrent release vs sweep and release restores once", func(t *testing.T) {
		repo, _ := newPGRepo(t)
		seedPGListing(t, repo, "L1", 1000)
		for i := 0; i < 30; i++ {
			id := fmt.Sprintf("race-%d", i)
			before := pgStock(t, repo, "L1")
			if err := repo.ReserveStockIdempotent(ctx, id, "L1", "", 4, time.Now().Add(-time.Minute)); err != nil {
				t.Fatalf("reserve: %v", err)
			}
			var wg sync.WaitGroup
			wg.Add(3)
			go func() { defer wg.Done(); _, _ = repo.ReleaseReservation(ctx, id) }()
			go func() { defer wg.Done(); _, _ = repo.ReleaseReservation(ctx, id) }()
			go func() { defer wg.Done(); _, _ = repo.SweepExpiredReservations(ctx, time.Now()) }()
			wg.Wait()
			if got := pgStock(t, repo, "L1"); got != before {
				t.Fatalf("iter %d: stock=%d want %d (restored exactly once)", i, got, before)
			}
		}
	})
}

func TestPG_ReserveStockIdempotent_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPGRepo(t)
	seedPGListing(t, repo, "L1", 10)

	if err := repo.ReserveStockIdempotent(ctx, "", "L1", "", 3, farFuture); !errors.Is(err, repository.ErrReservationIDRequired) {
		t.Fatalf("empty id: %v, want ErrReservationIDRequired", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 3, farFuture); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 3, farFuture); err != nil {
		t.Fatalf("repeat reserve: %v", err)
	}
	if got := pgStock(t, repo, "L1"); got != 7 {
		t.Fatalf("stock = %d, want 7 (decremented once)", got)
	}
	if _, err := repo.ReleaseReservation(ctx, "r1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 3, farFuture); !errors.Is(err, repository.ErrReservationReleased) {
		t.Fatalf("re-reserve released id: %v, want ErrReservationReleased", err)
	}
	if got := pgStock(t, repo, "L1"); got != 10 {
		t.Fatalf("stock = %d, want 10", got)
	}

	// Out of stock rolls the reservation row back: the id stays usable.
	if err := repo.ReserveStockIdempotent(ctx, "r2", "L1", "", 11, farFuture); !errors.Is(err, repository.ErrOutOfStock) {
		t.Fatalf("oversized reserve: %v, want ErrOutOfStock", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r2", "L1", "", 1, farFuture); err != nil {
		t.Fatalf("reserve after refused attempt: %v", err)
	}
	if got := pgStock(t, repo, "L1"); got != 9 {
		t.Fatalf("stock = %d, want 9", got)
	}
}

// Concurrent reserves of one id decrement once.
func TestPG_ReserveStockIdempotent_ConcurrentSameID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPGRepo(t)
	seedPGListing(t, repo, "L1", 100)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.ReserveStockIdempotent(ctx, "same", "L1", "", 5, farFuture)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("reserve #%d: %v", i, err)
		}
	}
	if got := pgStock(t, repo, "L1"); got != 95 {
		t.Fatalf("stock = %d, want 95", got)
	}
}
