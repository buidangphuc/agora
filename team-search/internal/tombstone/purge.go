// Package tombstone purges the read-model tombstones (D6) older than the
// retention, by processing time (when the indexer applied the delete). Only the
// indexer schedules it; the query server never purges.
package tombstone

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Purger is the read-model operation Purge drives (index.Index satisfies it).
type Purger interface {
	PurgeTombstones(ctx context.Context, olderThan time.Time) (int64, error)
}

// Purge removes tombstones older than now()-ttl and logs how many it removed. A
// failure is logged and returned wrapped; the scheduler keeps ticking after it.
func Purge(ctx context.Context, p Purger, ttl time.Duration, logger *slog.Logger, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	cutoff := now().Add(-ttl)
	n, err := p.PurgeTombstones(ctx, cutoff)
	if err != nil {
		logger.WarnContext(ctx, "tombstone purge failed", "err", err)
		return fmt.Errorf("purge tombstones: %w", err)
	}
	logger.InfoContext(ctx, "tombstone purge", "purged", n, "older_than", cutoff)
	return nil
}
