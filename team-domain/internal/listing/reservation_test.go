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

// TestSweepExpiredReservations proves SA-C2 (domain side): an ACTIVE reservation
// past its TTL is released by the sweeper and the stock restored, and the sweep
// is idempotent. Committed and released reservations are not swept (see
// TestSweepOnlyRestoresActiveReservations).
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

// Reserving under the id of a released reservation fails (the stock was given
// back) and leaves stock unchanged; active and committed ids stay idempotent.
func TestReserveStockIdempotent_ReleasedIDFails(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	svc := service.NewListingService(repo)

	if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 3); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := svc.CommitReservation(ctx, "r1"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 3); err != nil {
		t.Fatalf("re-reserve committed id: %v, want nil", err)
	}
	if got := stockOf(t, repo, "L1"); got != 7 {
		t.Fatalf("stock = %d, want 7", got)
	}
	if err := svc.ReleaseStock(ctx, "r1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 3); !errors.Is(err, repository.ErrReservationReleased) {
		t.Fatalf("re-reserve released id: %v, want ErrReservationReleased", err)
	}
	if got := stockOf(t, repo, "L1"); got != 10 {
		t.Fatalf("stock = %d, want 10", got)
	}
}

func TestReserveStockIdempotent_EmptyIDRequired(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	svc := service.NewListingService(repo)
	if err := svc.ReserveStockIdempotent(context.Background(), "", "L1", "", 3); !errors.Is(err, repository.ErrReservationIDRequired) {
		t.Fatalf("got %v, want ErrReservationIDRequired", err)
	}
	if got := stockOf(t, repo, "L1"); got != 10 {
		t.Fatalf("stock = %d, want 10", got)
	}
}

// The sweep restores only ACTIVE reservations past their TTL: a committed one (a
// placed order) keeps its stock, and an explicitly released one is not restored
// a second time.
func TestSweepOnlyRestoresActiveReservations(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	svc := service.NewListingService(repo)
	sweeper := service.NewReservationSweeper(svc, 0, nil)

	for id, qty := range map[string]int32{"active": 1, "committed": 2, "released": 3} {
		if err := svc.ReserveStockIdempotent(ctx, id, "L1", "", qty); err != nil {
			t.Fatalf("reserve %s: %v", id, err)
		}
	}
	if err := svc.CommitReservation(ctx, "committed"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := svc.ReleaseStock(ctx, "released"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := stockOf(t, repo, "L1"); got != 7 {
		t.Fatalf("before sweep stock = %d, want 7", got)
	}

	released, err := sweeper.SweepOnce(ctx, time.Now().Add(service.DefaultReservationTTL+time.Minute))
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1 (only the active reservation)", released)
	}
	if got := stockOf(t, repo, "L1"); got != 8 {
		t.Errorf("after sweep stock = %d, want 8 (committed 2 still held)", got)
	}
	// The swept id is now released: reserving under it fails, committing it fails.
	if err := svc.ReserveStockIdempotent(ctx, "active", "L1", "", 1); !errors.Is(err, repository.ErrReservationReleased) {
		t.Errorf("re-reserve swept id: %v, want ErrReservationReleased", err)
	}
	if err := svc.CommitReservation(ctx, "active"); !errors.Is(err, repository.ErrReservationReleased) {
		t.Errorf("commit swept id: %v, want ErrReservationReleased", err)
	}
}
