package query

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// GA4TypeCount is one event type's same-day counts from GA4 and first-party.
type GA4TypeCount struct {
	EventType  string
	GA4        int64
	FirstParty int64
}

// GA4Reconcile is the per-day reconciliation input. ImportRun is false when
// the run ledger has no row for the day.
type GA4Reconcile struct {
	ImportRun bool
	Types     []GA4TypeCount
}

// GA4ReconcileRepository is the optional repository capability for reconciliation.
type GA4ReconcileRepository interface {
	GA4Reconcile(ctx context.Context, day time.Time) (GA4Reconcile, error)
}

// ga4EventTypes maps first-party event types to GA4 event names (purchase excluded).
var ga4EventTypes = []struct{ eventType, ga4Name string }{
	{"view", "view_item"},
	{"click", "select_item"},
	{"add_to_cart", "add_to_cart"},
	{"begin_checkout", "begin_checkout"},
}

// GA4Reconcile counts GA4 and first-party events for one UTC day.
func (r *DuckDBRepository) GA4Reconcile(ctx context.Context, day time.Time) (GA4Reconcile, error) {
	var out GA4Reconcile
	day = day.UTC().Truncate(24 * time.Hour)
	next := day.AddDate(0, 0, 1)
	var runs int64
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE export_date = CAST(? AS DATE)", warehouse.GA4ImportRunsTableName), day.Format("2006-01-02")).Scan(&runs); err != nil {
		return out, fmt.Errorf("ga4 import runs: %w", err)
	}
	out.ImportRun = runs > 0
	for _, m := range ga4EventTypes {
		var c GA4TypeCount
		c.EventType = m.eventType
		if err := r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COALESCE(SUM(events),0) FROM %s WHERE day = CAST(? AS DATE) AND event_name = ?", warehouse.GA4EventsDailyTableName), day.Format("2006-01-02"), m.ga4Name).Scan(&c.GA4); err != nil {
			return out, fmt.Errorf("ga4 events: %w", err)
		}
		if err := r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE event_type = ? AND occurred_at >= ? AND occurred_at < ?", warehouse.ResolvedViewName), m.eventType, day, next).Scan(&c.FirstParty); err != nil {
			return out, fmt.Errorf("first-party events: %w", err)
		}
		out.Types = append(out.Types, c)
	}
	return out, nil
}
