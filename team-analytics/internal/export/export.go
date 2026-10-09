// Package export periodically writes the warehouse's tracking_events table, plus
// the feature inputs tracking_events_resolved, engagement_facts and order_facts,
// to Parquet files for offline jobs (platform-recsys, platform-featurestore). DuckDB holds an exclusive
// file lock, so an in-process export is the only safe way for another process to
// read the data. Each export goes to a temporary file that is then renamed over
// the target, so a reader never sees a partial file.
package export

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Exporter writes a warehouse table or view to dst as Parquet (duckdb.Writer).
type Exporter interface {
	ExportRelation(ctx context.Context, name, dst string) error
}

// Target is one exported relation and the file it lands in.
type Target struct {
	Relation string
	Path     string
}

// Targets lists every file an export cycle writes: the tracking table at path,
// and the three feature inputs beside it, named <relation>.parquet.
func Targets(path string) []Target {
	dir := filepath.Dir(path)
	ts := []Target{{warehouse.TableName, path}}
	for _, rel := range []string{warehouse.ResolvedViewName, warehouse.EngagementFactsTableName, warehouse.OrderFactsTableName} {
		ts = append(ts, Target{rel, filepath.Join(dir, rel+".parquet")})
	}
	return ts
}

// Enabled reports whether the export is configured: a path and a positive interval.
func Enabled(path string, intervalSeconds int) bool {
	return path != "" && intervalSeconds > 0
}

// Once exports one relation to path atomically: write path+".tmp", then rename
// over path.
func Once(ctx context.Context, e Exporter, relation, path string) error {
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %q: %w", tmp, err)
	}
	if err := e.ExportRelation(ctx, relation, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}

// Cycle writes every target atomically. A failure on one file is logged and does
// not block the others; the first error is returned.
func Cycle(ctx context.Context, e Exporter, path string, logger *slog.Logger) error {
	var first error
	for _, t := range Targets(path) {
		if err := Once(ctx, e, t.Relation, t.Path); err != nil {
			if ctx.Err() != nil {
				return err
			}
			logger.WarnContext(ctx, "parquet export failed", slog.String("relation", t.Relation),
				slog.String("path", t.Path), slog.Any("err", err))
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// Run exports once at start and then every interval until ctx is done. A failed
// file is logged and retried on the next tick; it never stops the caller.
func Run(ctx context.Context, e Exporter, path string, interval time.Duration, logger *slog.Logger) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := Cycle(ctx, e, path, logger); err != nil && ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
