package engagement

import (
	"context"
	"time"
)

type ReportSweeper interface {
	SweepClosedBefore(ctx context.Context, cutoff time.Time, batch int) (int64, error)
}

const reportSweepBatch = 500

func SweepReports(ctx context.Context, repo ReportSweeper, cutoff time.Time) (int64, error) {
	var total int64
	for {
		n, err := repo.SweepClosedBefore(ctx, cutoff, reportSweepBatch)
		total += n
		if err != nil {
			return total, err
		}
		if n < reportSweepBatch {
			return total, nil
		}
	}
}
