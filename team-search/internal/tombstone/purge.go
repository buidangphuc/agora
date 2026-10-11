// Package tombstone runs the indexer's tombstone purge loop (D6): every
// interval it removes the read-model tombstones older than the retention, by
// processing time (when the indexer applied the delete). Only the indexer runs
// it; the query server never purges.
package tombstone

import (
	"context"
	"log/slog"
	"time"
)

// Purger is the read-model operation the loop drives (index.Index satisfies it).
type Purger interface {
	PurgeTombstones(ctx context.Context, olderThan time.Time) (int64, error)
}

// Loop purges expired tombstones every Interval. Now and After default to the
// wall clock; tests inject a fake clock.
type Loop struct {
	Purger   Purger
	TTL      time.Duration
	Interval time.Duration
	Logger   *slog.Logger
	Now      func() time.Time
	After    func(time.Duration) <-chan time.Time
}

// Run blocks until ctx is done. A failed purge is logged and retried at the next
// tick; it never stops the indexer.
func (l Loop) Run(ctx context.Context) {
	now, after := l.Now, l.After
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = time.After
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-after(l.Interval):
		}
		cutoff := now().Add(-l.TTL)
		n, err := l.Purger.PurgeTombstones(ctx, cutoff)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			l.Logger.Warn("tombstone purge failed", slog.Any("err", err))
			continue
		}
		l.Logger.Info("tombstone purge", slog.Int64("purged", n), slog.Time("older_than", cutoff))
	}
}
