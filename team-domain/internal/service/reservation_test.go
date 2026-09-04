package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// stockOf is a small helper: the current base stock of listing id.
func stockOf(t *testing.T, repo *repository.InMemoryListingRepository, id string) int32 {
	t.Helper()
	l, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return l.Stock
}

// TestReserveStockIdempotent reproduces SA-M6: a retried checkout that reuses the
// same reservation_id must decrement stock exactly ONCE.
func TestReserveStockIdempotent(t *testing.T) {
	ctx := context.Background()

	t.Run("double reserve with same reservation_id decrements once", func(t *testing.T) {
		repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
		svc := service.NewListingService(repo)

		if err := svc.ReserveStockIdempotent(ctx, "res-1", "L1", "", 3); err != nil {
			t.Fatalf("first reserve: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 7 {
			t.Fatalf("after first reserve stock = %d, want 7", got)
		}

		// Retry with the SAME reservation_id: no-op, stock unchanged.
		if err := svc.ReserveStockIdempotent(ctx, "res-1", "L1", "", 3); err != nil {
			t.Fatalf("retry reserve: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 7 {
			t.Errorf("after retry stock = %d, want 7 (must decrement once)", got)
		}

		// A DIFFERENT reservation_id is a real, distinct reserve.
		if err := svc.ReserveStockIdempotent(ctx, "res-2", "L1", "", 2); err != nil {
			t.Fatalf("second distinct reserve: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 5 {
			t.Errorf("after distinct reserve stock = %d, want 5", got)
		}
	})

	t.Run("insufficient stock is refused and records no reservation", func(t *testing.T) {
		repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L2", Currency: "VND", Stock: 1})
		svc := service.NewListingService(repo)

		if err := svc.ReserveStockIdempotent(ctx, "res-x", "L2", "", 5); !errors.Is(err, repository.ErrOutOfStock) {
			t.Fatalf("want ErrOutOfStock, got %v", err)
		}
		if got := stockOf(t, repo, "L2"); got != 1 {
			t.Errorf("stock must be untouched on failure, got %d want 1", got)
		}
		// Because no reservation was recorded, a later reserve of the SAME id with a
		// now-satisfiable quantity must still decrement.
		if err := svc.ReserveStockIdempotent(ctx, "res-x", "L2", "", 1); err != nil {
			t.Fatalf("reserve after prior failure: %v", err)
		}
		if got := stockOf(t, repo, "L2"); got != 0 {
			t.Errorf("stock = %d, want 0", got)
		}
	})
}

// TestSweepExpiredReservations proves SA-C2 (domain side): a reservation past its
// TTL is released by the sweeper and the stock restored, and the sweep is
// idempotent.
func TestSweepExpiredReservations(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	svc := service.NewListingService(repo)
	sweeper := service.NewReservationSweeper(svc, 0, nil)

	if err := svc.ReserveStockIdempotent(ctx, "res-1", "L1", "", 4); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := stockOf(t, repo, "L1"); got != 6 {
		t.Fatalf("after reserve stock = %d, want 6", got)
	}

	// Nothing is expired yet at the real current time.
	if released, err := sweeper.SweepOnce(ctx, time.Now()); err != nil || released != 0 {
		t.Fatalf("premature sweep released=%d err=%v, want 0/nil", released, err)
	}
	if got := stockOf(t, repo, "L1"); got != 6 {
		t.Errorf("stock changed by premature sweep: %d", got)
	}

	// Advance past the TTL: the reservation is released and stock restored.
	future := time.Now().Add(service.DefaultReservationTTL + time.Minute)
	released, err := sweeper.SweepOnce(ctx, future)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1", released)
	}
	if got := stockOf(t, repo, "L1"); got != 10 {
		t.Errorf("stock after sweep = %d, want 10 (restored)", got)
	}

	// Idempotent: a second sweep releases nothing and leaves stock alone.
	released, err = sweeper.SweepOnce(ctx, future)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if released != 0 {
		t.Errorf("second sweep released = %d, want 0", released)
	}
	if got := stockOf(t, repo, "L1"); got != 10 {
		t.Errorf("stock after second sweep = %d, want 10", got)
	}
}
