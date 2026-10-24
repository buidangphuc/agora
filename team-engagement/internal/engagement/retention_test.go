package engagement_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

// recordingSweeper wraps a real in-memory repository and records every call.
type recordingSweeper struct {
	inner   *storetest.FakeReportRepository
	mu      sync.Mutex
	batches []int
	cutoffs []time.Time
	failOn  int // 1-based call index that fails (0 = never)
}

func (r *recordingSweeper) SweepClosedBefore(ctx context.Context, cutoff time.Time, batch int) (int64, error) {
	r.mu.Lock()
	r.batches = append(r.batches, batch)
	r.cutoffs = append(r.cutoffs, cutoff)
	n := len(r.batches)
	fail := r.failOn != 0 && n == r.failOn
	r.mu.Unlock()
	if fail {
		return 0, errors.New("boom")
	}
	return r.inner.SweepClosedBefore(ctx, cutoff, batch)
}

func seedClosed(repo *storetest.FakeReportRepository, n int, resolved time.Time) {
	for i := 0; i < n; i++ {
		ts := resolved
		repo.SeedReport(store.Report{
			ID: fmt.Sprintf("r-%05d", i), ReporterID: "u-1", Note: "n",
			Status: store.ReportStatusActioned, ResolvedAt: &ts,
		})
	}
}

func countRedacted(t *testing.T, repo *storetest.FakeReportRepository) int {
	t.Helper()
	got, _, err := repo.ListReports(context.Background(), "", "", 0, 5000)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range got {
		if r.ReporterID == store.ReportRedacted && r.Note == "" {
			n++
		}
	}
	return n
}

// Scenario: RT Listing reports: Sweep is idempotent and batched.
func TestSweepReportsIdempotentAndBatched(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	repo := storetest.NewFakeReportRepository()
	seedClosed(repo, 1200, now.AddDate(0, 0, -731))
	rec := &recordingSweeper{inner: repo}

	got, err := engagement.SweepReports(context.Background(), rec, now.AddDate(0, 0, -730))
	if err != nil || got != 1200 {
		t.Fatalf("first run = %d, %v; want 1200, nil", got, err)
	}
	if len(rec.batches) != 3 {
		t.Fatalf("first run made %d batch calls, want 3", len(rec.batches))
	}
	for _, b := range rec.batches {
		if b <= 0 || b > 500 {
			t.Errorf("batch size %d, want 1..500", b)
		}
	}
	if n := countRedacted(t, repo); n != 1200 {
		t.Errorf("redacted rows = %d, want 1200", n)
	}

	got, err = engagement.SweepReports(context.Background(), rec, now.AddDate(0, 0, -730))
	if err != nil || got != 0 {
		t.Errorf("second run = %d, %v; want 0, nil", got, err)
	}
}

// Scenario: RT Listing reports: Sweep is idempotent and batched (failure in batch 2).
func TestSweepReportsFailureKeepsEarlierBatchAndResumes(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	repo := storetest.NewFakeReportRepository()
	seedClosed(repo, 1200, now.AddDate(0, 0, -731))
	rec := &recordingSweeper{inner: repo, failOn: 2}
	cutoff := now.AddDate(0, 0, -730)

	got, err := engagement.SweepReports(context.Background(), rec, cutoff)
	if err == nil {
		t.Fatal("expected the batch-2 failure to be returned")
	}
	if got != 500 {
		t.Errorf("rows reported before failure = %d, want 500", got)
	}
	if n := countRedacted(t, repo); n != 500 {
		t.Fatalf("batch 1 must stay committed: redacted = %d, want 500", n)
	}

	got, err = engagement.SweepReports(context.Background(), rec, cutoff)
	if err != nil || got != 700 {
		t.Errorf("resume run = %d, %v; want 700, nil", got, err)
	}
	if n := countRedacted(t, repo); n != 1200 {
		t.Errorf("redacted after resume = %d, want 1200", n)
	}
}

// Scenario: RT Listing reports: Sweep is idempotent and batched (nothing eligible).
func TestSweepReportsNothingEligible(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	repo := storetest.NewFakeReportRepository()
	seedClosed(repo, 3, now.AddDate(0, 0, -10))
	got, err := engagement.SweepReports(context.Background(), &recordingSweeper{inner: repo}, now.AddDate(0, 0, -730))
	if err != nil || got != 0 {
		t.Errorf("got %d, %v; want 0, nil", got, err)
	}
	if n := countRedacted(t, repo); n != 0 {
		t.Errorf("recent reports must stay untouched, redacted = %d", n)
	}
}
