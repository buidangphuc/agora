// Package export periodically writes the warehouse's tracking_events table, plus
// the feature inputs tracking_events_resolved, engagement_facts and order_facts,
// to Parquet files for offline jobs (platform-recsys, platform-featurestore). DuckDB holds an exclusive
// file lock, so an in-process export is the only safe way for another process to
// read the data. Each export goes to a temporary file that is then renamed over
// the target, so a reader never sees a partial file.
package export

import (
	"context"
	"database/sql"
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
// and the feature inputs beside it, named <relation>.parquet (listing_sellers carries each
// listing's seller, category and price).
func Targets(path string) []Target {
	dir := filepath.Dir(path)
	ts := []Target{{warehouse.TableName, path}}
	for _, rel := range []string{warehouse.ResolvedViewName, warehouse.EngagementFactsTableName, warehouse.OrderFactsTableName, warehouse.ListingSellersTableName, warehouse.GA4SessionsTableName, warehouse.AttributedViewName} {
		ts = append(ts, Target{rel, filepath.Join(dir, rel+".parquet")})
	}
	return ts
}

// Enabled reports whether the export is configured: a path and a positive interval.
func Enabled(path string, intervalSeconds int) bool {
	return path != "" && intervalSeconds > 0
}

// trackingWithSessionContext is a COPY source (a parenthesised query, accepted
// wherever COPY takes a relation name) that adds session_source, session_medium
// and session_campaign to every tracking event. Rule: the latest ga4_sessions row
// of the same user (ga_client_id = user_pseudo_id) that started at or before the
// event (ties broken by session_id), so an event never duplicates; empty strings
// when ga_client_id is empty or no session matches. GA4 is only ever read as
// context, never as an event source: the rows come from tracking_events alone.
const trackingWithSessionContext = `(SELECT t.*,
  COALESCE(s.source, '') AS session_source,
  COALESCE(s.medium, '') AS session_medium,
  COALESCE(s.campaign, '') AS session_campaign
FROM ` + warehouse.TableName + ` t
LEFT JOIN LATERAL (
  SELECT source, medium, campaign FROM ` + warehouse.GA4SessionsTableName + ` g
  WHERE t.ga_client_id <> '' AND g.user_pseudo_id = t.ga_client_id AND g.ts <= t.occurred_at
  ORDER BY g.ts DESC, g.session_id DESC LIMIT 1
) s ON true)`

// Once exports one relation to path atomically: write path+".tmp", then rename
// over path.
//
// The tracking table is exported through trackingWithSessionContext so each event
// also carries session_source/medium/campaign from ga4_sessions.
func Once(ctx context.Context, e Exporter, relation, path string) error {
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %q: %w", tmp, err)
	}
	var err error
	if db, ok := e.(interface{ DB() *sql.DB }); ok && relation == warehouse.TableName {
		_, err = db.DB().ExecContext(ctx, fmt.Sprintf("COPY %s TO '%s' (FORMAT PARQUET)", trackingWithSessionContext, tmp))
	} else {
		err = e.ExportRelation(ctx, relation, tmp)
	}
	if err != nil {
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
