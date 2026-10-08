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
