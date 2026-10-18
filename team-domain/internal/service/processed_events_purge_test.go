package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// purgeRepo is an in-memory repository that also implements the optional
// processed-events purge capability the sweeper discovers by type assertion:
//
//	PurgeProcessedEvents(ctx context.Context, before time.Time) (int, error)
type purgeRepo struct {
	*repository.InMemoryListingRepository
	cutoffs []time.Time
}

func (p *purgeRepo) PurgeProcessedEvents(_ context.Context, before time.Time) (int, error) {
	p.cutoffs = append(p.cutoffs, before)
	return 3, nil
}

// Seller hold: Consumer disabled does nothing -- retention is 14 days.
func TestProcessedEventsRetentionIs14Days(t *testing.T) {
	if service.ProcessedEventsRetention != 14*24*time.Hour {
		t.Errorf("ProcessedEventsRetention = %v, want 336h", service.ProcessedEventsRetention)
	}
}

// Seller hold: Consumer disabled does nothing -- a sweep purges processed_events
// older than now-14d.
func TestSweepOncePurgesProcessedEventsOlderThan14Days(t *testing.T) {
	repo := &purgeRepo{InMemoryListingRepository: repository.NewInMemoryListingRepository()}
	svc := service.NewListingService(repo)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if _, err := service.NewReservationSweeper(svc, time.Minute, nil).SweepOnce(context.Background(), now); err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if len(repo.cutoffs) != 1 {
		t.Fatalf("purge called %d times, want 1", len(repo.cutoffs))
	}
	if want := now.Add(-14 * 24 * time.Hour); !repo.cutoffs[0].Equal(want) {
		t.Errorf("purge cutoff = %v, want %v", repo.cutoffs[0], want)
	}
}

// Seller hold: Consumer disabled does nothing -- a repository without the purge
// capability (in-memory) keeps sweeping reservations unaffected.
func TestSweepOnceWithoutPurgeCapabilityStillSweeps(t *testing.T) {
	svc := service.NewListingService(repository.NewInMemoryListingRepository())
	if _, err := service.NewReservationSweeper(svc, time.Minute, nil).SweepOnce(context.Background(), time.Now()); err != nil {
		t.Errorf("SweepOnce: %v", err)
	}
}
