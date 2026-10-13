// Package ga4import loads one day of the GA4 BigQuery export into ga4_sessions,
// ga4_events_daily and ga4_import_runs (add-ga4-export-import). A Source
// aggregates the raw events; Run replaces the day's rows in one transaction.
package ga4import

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/team-analytics/internal/store"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// Session is one (user_pseudo_id, session_id) row; LandingPage has no query string.
type Session struct {
	UserPseudoID, SessionID, Source, Medium, Campaign, LandingPage string
	Start, End                                                     time.Time
}

// DailyCount is the number of GA4 events of one name on the imported day.
type DailyCount struct {
	EventName string
	Events    int64
}

// Extract is a Source's aggregated view of one export day.
type Extract struct {
	Events   int64 // raw event rows read
	Sessions []Session
	Daily    []DailyCount
}

// Source reads the export table of one day. It must fail (naming the date) when
// the table does not exist, and return an empty Extract for an empty table.
type Source interface {
	Extract(ctx context.Context, day time.Time) (*Extract, error)
}

// Result reports what Run imported.
type Result struct {
	Rows int64 // events read from the export
}

// Run replaces all GA4 rows of day in one transaction. On any error nothing changes.
func Run(ctx context.Context, w *duckdb.Writer, src Source, day time.Time) (Result, error) {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	ex, err := src.Extract(ctx, day)
	if err != nil {
		return Result{}, fmt.Errorf("ga4 export %s: %w", day.Format("2006-01-02"), err)
	}
	tx, err := w.DB().BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, t := range []struct{ table, col string }{
		{store.GA4SessionsTableName, "export_date"},
		{store.GA4EventsDailyTableName, "day"},
		{store.GA4ImportRunsTableName, "export_date"},
	} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t.table, t.col), day); err != nil {
			return Result{}, fmt.Errorf("replace %s: %w", t.table, err)
		}
	}
	for _, s := range ex.Sessions {
		if _, err := tx.ExecContext(ctx, "INSERT INTO ga4_sessions (user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, export_date, ended_at) VALUES (?,?,?,?,?,?,?,?,?)",
			s.UserPseudoID, s.SessionID, s.Source, s.Medium, s.Campaign, s.LandingPage, s.Start.UTC(), day, s.End.UTC()); err != nil {
			return Result{}, fmt.Errorf("insert ga4_sessions: %w", err)
		}
	}
	for _, d := range ex.Daily {
		if _, err := tx.ExecContext(ctx, "INSERT INTO ga4_events_daily (day, event_name, events) VALUES (?,?,?)", day, d.EventName, d.Events); err != nil {
			return Result{}, fmt.Errorf("insert ga4_events_daily: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ga4_import_runs (export_date, rows, finished_at) VALUES (?,?,?)", day, ex.Events, time.Now().UTC()); err != nil {
		return Result{}, fmt.Errorf("insert ga4_import_runs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{Rows: ex.Events}, nil
}
