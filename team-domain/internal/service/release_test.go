package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

func newReleaseSvc(t *testing.T, stock int32) (*service.ListingService, *repository.InMemoryListingRepository) {
	t.Helper()
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: stock})
	return service.NewListingService(repo), repo
}

// Release is keyed by reservation_id and restores the STORED quantity once.
func TestReleaseStock_ByReservationID(t *testing.T) {
	ctx := context.Background()

	t.Run("double release restores once", func(t *testing.T) {
		svc, repo := newReleaseSvc(t, 10)
		if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 3); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		for i := 0; i < 2; i++ {
			if err := svc.ReleaseStock(ctx, "r1"); err != nil {
				t.Fatalf("release #%d: %v", i+1, err)
			}
		}
		if got := stockOf(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("release after sweep is a no-op", func(t *testing.T) {
		svc, repo := newReleaseSvc(t, 10)
		if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 3); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if n, _ := svc.SweepExpiredReservations(ctx, time.Now().Add(service.DefaultReservationTTL+time.Minute)); n != 1 {
			t.Fatalf("sweep released %d, want 1", n)
		}
		if err := svc.ReleaseStock(ctx, "r1"); err != nil {
			t.Fatalf("release: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10 (restored once)", got)
		}
	})

	t.Run("unknown id is a no-op", func(t *testing.T) {
		svc, repo := newReleaseSvc(t, 10)
		if err := svc.ReleaseStock(ctx, "never-reserved"); err != nil {
			t.Fatalf("release: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("committed reservation is releasable", func(t *testing.T) {
		svc, repo := newReleaseSvc(t, 10)
		if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 2); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if err := svc.CommitReservation(ctx, "r1"); err != nil {
			t.Fatalf("commit: %v", err)
		}
		if err := svc.ReleaseStock(ctx, "r1"); err != nil {
			t.Fatalf("release: %v", err)
		}
		if got := stockOf(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})

	t.Run("concurrent release vs sweep restores once", func(t *testing.T) {
		svc, repo := newReleaseSvc(t, 10)
		if err := svc.ReserveStockIdempotent(ctx, "r1", "L1", "", 4); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); _ = svc.ReleaseStock(ctx, "r1") }()
			go func() {
				defer wg.Done()
				_, _ = svc.SweepExpiredReservations(ctx, time.Now().Add(service.DefaultReservationTTL+time.Minute))
			}()
		}
		wg.Wait()
		if got := stockOf(t, repo, "L1"); got != 10 {
			t.Fatalf("stock = %d, want 10", got)
		}
	})
}
