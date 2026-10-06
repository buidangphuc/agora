// Package export periodically writes the warehouse's tracking_events table to a
// Parquet file for offline training (platform-recsys). DuckDB holds an exclusive
// file lock, so an in-process export is the only safe way for another process to
// read the data. Each export goes to a temporary file that is then renamed over
// the target, so a reader never sees a partial file.
package export

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Exporter writes the tracking table to dst as Parquet (duckdb.Writer).
type Exporter interface {
	ExportParquet(ctx context.Context, dst string) error
}

// Enabled reports whether the export is configured: a path and a positive interval.
func Enabled(path string, intervalSeconds int) bool {
	return path != "" && intervalSeconds > 0
}

// Once exports to path atomically: write path+".tmp", then rename over path.
func Once(ctx context.Context, e Exporter, path string) error {
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %q: %w", tmp, err)
	}
	if err := e.ExportParquet(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}

// Run exports once at start and then every interval until ctx is done. A failed
// export is logged and retried on the next tick; it never stops the caller.
func Run(ctx context.Context, e Exporter, path string, interval time.Duration, logger *slog.Logger) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := Once(ctx, e, path); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.WarnContext(ctx, "parquet export failed", slog.String("path", path), slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
