package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Retention deletes published outbox rows older than a window, so the table does
// not grow without bound. Pending rows (work) and parked rows (evidence of a
// poison event) are never touched. It is a lifecycle component.
type Retention struct {
	pool     *pgxpool.Pool
	table    string
	keep     time.Duration
	interval time.Duration
	logger   *slog.Logger
}

// NewRetention returns a component that deletes rows of table published more than
// keep ago, once at start and then every interval. table must be a trusted
// identifier (outbox tables are named in code).
func NewRetention(pool *pgxpool.Pool, table string, keep, interval time.Duration, logger *slog.Logger) *Retention {
	if logger == nil {
		logger = slog.Default()
	}
	return &Retention{pool: pool, table: table, keep: keep, interval: interval, logger: logger}
}

// Sweep deletes the expired published rows and returns how many it removed.
func (r *Retention) Sweep(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM `+r.table+` WHERE status = 'published' AND published_at < now() - $1::interval`,
		fmt.Sprintf("%d milliseconds", r.keep.Milliseconds()))
	if err != nil {
		return 0, fmt.Errorf("trim %s: %w", r.table, err)
	}
	return tag.RowsAffected(), nil
}

// Run sweeps at start and then on every tick until ctx is cancelled, then returns
// nil. A failed sweep is logged and retried on the next tick.
func (r *Retention) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		n, err := r.Sweep(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			r.logger.WarnContext(ctx, "outbox retention sweep failed", "table", r.table, "err", err)
		case n > 0:
			r.logger.InfoContext(ctx, "outbox retention removed published rows", "table", r.table, "removed", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
