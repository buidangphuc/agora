// Package tombstone runs the indexer's tombstone purge loop (D6): every
// interval it removes the read-model tombstones older than the retention, by
// processing time (when the indexer applied the delete). Only the indexer runs
// it; the query server never purges.
package tombstone

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
)

// Purger is the read-model operation the loop drives (index.Index satisfies it).
type Purger interface {
	PurgeTombstones(ctx context.Context, olderThan time.Time) (int64, error)
}

// Loop purges expired tombstones every Interval. Now defaults to the wall
// clock; tests inject a fake clock.
type Loop struct {
	Purger   Purger
	TTL      time.Duration
	Interval time.Duration
	Logger   *slog.Logger
	Now      func() time.Time
}

// Component returns the periodic purge as a lifecycle component (lifecycle.Every
// owns the ticker). It fails when Interval is not positive.
func (l Loop) Component() (lifecycle.Component, error) {
	return lifecycle.Every(l.Interval, l.purge, lifecycle.WithLogger(l.Logger))
}

// purge removes the tombstones older than now - TTL. A failed purge is logged
// and retried at the next tick; it never stops the indexer.
func (l Loop) purge(ctx context.Context) error {
	now := l.Now
	if now == nil {
		now = time.Now
	}
	cutoff := now().Add(-l.TTL)
	n, err := l.Purger.PurgeTombstones(ctx, cutoff)
	if err != nil {
		if ctx.Err() == nil {
			l.Logger.WarnContext(ctx, "tombstone purge failed", "err", err)
		}
		return nil
	}
	l.Logger.InfoContext(ctx, "tombstone purge", "purged", n, "older_than", cutoff)
	return nil
}
