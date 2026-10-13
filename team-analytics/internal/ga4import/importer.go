package ga4import

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/store/duckdb"
)

// Importer runs the GA4 export import through the consumer's own writer
// (single DuckDB writer; no second connection).
type Importer struct {
	cfg config.GA4
	w   *duckdb.Writer
	// bqProject is the BigQuery project of the export (BIGQUERY_PROJECT); empty falls back to the dataset's "project." prefix.
	bqProject string
	mu        sync.Mutex // a manual trigger and the scheduled run must not overlap
}

// NewImporter returns an Importer over cfg that writes through w.
func NewImporter(cfg config.GA4, w *duckdb.Writer) *Importer {
	return &Importer{cfg: cfg, w: w}
}

// SetBigQueryProject sets the BigQuery project of the export (BIGQUERY_PROJECT).
func (g *Importer) SetBigQueryProject(project string) { g.bqProject = project }

// ImportDay imports one export day; 0,nil when nothing is configured.
func (g *Importer) ImportDay(ctx context.Context, day time.Time) (int64, error) {
	if strings.TrimSpace(g.cfg.ExportDataset) == "" && strings.TrimSpace(g.cfg.FixtureDir) == "" {
		return 0, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var src Source
	if g.cfg.ExportSource == "fixture" {
		if g.cfg.FixtureDir == "" {
			return 0, fmt.Errorf("ga4 export %s: GA4_EXPORT_FIXTURE_DIR is required when GA4_EXPORT_SOURCE=fixture", day.Format("2006-01-02"))
		}
		src = NewFixtureSource(g.cfg.FixtureDir)
	} else {
		bq, err := NewBigQuerySource(ctx, g.bqProject, g.cfg.ExportDataset)
		if err != nil {
			return 0, fmt.Errorf("ga4 export %s: bigquery source: %w", day.Format("2006-01-02"), err)
		}
		defer bq.Close()
		src = bq
	}
	res, err := Run(ctx, g.w, src, day)
	if err != nil {
		return 0, err
	}
	return res.Rows, nil
}

// TriggerHandler serves POST ?date=YYYY-MM-DD.
func TriggerHandler(imp *Importer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		raw := r.URL.Query().Get("date")
		day, err := time.Parse("2006-01-02", raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid date %q: want YYYY-MM-DD", raw), http.StatusBadRequest)
			return
		}
		rows, err := imp.ImportDay(r.Context(), day)
		if err != nil {
			msg := err.Error()
			if !strings.Contains(msg, day.Format("2006-01-02")) {
				msg = day.Format("2006-01-02") + ": " + msg
			}
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":` + strconv.FormatInt(rows, 10) + `}`))
	})
}

// NextRun is the next UTC "HH:MM" strictly after now.
func NextRun(now time.Time, at string) (time.Time, error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, fmt.Errorf("GA4_IMPORT_AT %q: want HH:MM: %w", at, err)
	}
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next, nil
}

// PreviousDay is the previous UTC date at 00:00.
func PreviousDay(now time.Time) time.Time {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}

// RunScheduler imports yesterday daily at `at` (UTC) until ctx is done.
func RunScheduler(ctx context.Context, imp *Importer, at string, logger *slog.Logger) {
	for {
		next, err := NextRun(time.Now(), at)
		if err != nil {
			logger.ErrorContext(ctx, "ga4 import scheduler disabled", "err", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
		day := PreviousDay(time.Now())
		rows, err := imp.ImportDay(ctx, day)
		if err != nil {
			logger.ErrorContext(ctx, "ga4 import failed", "date", day.Format("2006-01-02"), "err", err)
			continue
		}
		logger.InfoContext(ctx, "ga4 import done", "date", day.Format("2006-01-02"), "rows", rows)
	}
}
