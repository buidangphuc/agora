package bootstrap

// TEST-STUB: compile-only placeholder for task 5; the implementer replaces this
// file (delete it) with the real sweep in a non-stub file.

import (
	"context"
	"log/slog"
	"time"
)

type ReportSweeper interface {
	SweepClosedBefore(ctx context.Context, cutoff time.Time, batch int) (int64, error)
}

func SweepReports(ctx context.Context, repo ReportSweeper, cutoff time.Time) (int64, error) {
	return 0, nil
}

func RunReportSweepLoop(ctx context.Context, repo ReportSweeper, retention time.Duration, ticks <-chan time.Time, now func() time.Time, logger *slog.Logger) {
}
