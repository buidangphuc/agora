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

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const dateLayout = "2006-01-02"

type Importer struct {
	cfg       config.GA4
	mocks     *mockdata.Set
	w         *duckdb.Writer
	bqProject string
	mu        sync.Mutex
}

func NewImporter(cfg config.GA4, mocks *mockdata.Set, w *duckdb.Writer) *Importer {
	return &Importer{cfg: cfg, mocks: mocks, w: w}
}

func (g *Importer) SetBigQueryProject(project string) { g.bqProject = project }

func (g *Importer) ImportDay(ctx context.Context, day time.Time) (int64, error) {
	mock := g.mocks.Active("ga4export")
	fixtureDir := strings.TrimSpace(g.cfg.FixtureDir)
	dataset := strings.TrimSpace(g.cfg.ExportDataset)
	if mock && fixtureDir == "" {
		return 0, fmt.Errorf("ga4 export %s: GA4_EXPORT_FIXTURE_DIR is required when mock data enables ga4export", day.Format(dateLayout))
	}
	if !mock && dataset == "" {
		return 0, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var src Source
	if mock {
		src = NewFixtureSource(fixtureDir)
	} else {
		bq, err := NewBigQuerySource(ctx, g.bqProject, dataset)
		if err != nil {
			return 0, fmt.Errorf("ga4 export %s: bigquery source: %w", day.Format(dateLayout), err)
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

func TriggerHandler(imp *Importer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		raw := r.URL.Query().Get("date")
		day, err := time.Parse(dateLayout, raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid date %q: want YYYY-MM-DD", raw), http.StatusBadRequest)
			return
		}
		rows, err := imp.ImportDay(r.Context(), day)
		if err != nil {
			msg := err.Error()
			if !strings.Contains(msg, day.Format(dateLayout)) {
				msg = day.Format(dateLayout) + ": " + msg
			}
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":` + strconv.FormatInt(rows, 10) + `}`))
	})
}

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

func ImportDate(now time.Time) time.Time {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}

func (g *Importer) RunScheduler(ctx context.Context, logger *slog.Logger) {
	for {
		next, err := NextRun(time.Now(), g.cfg.ImportAt)
		if err != nil {
			logger.Error("ga4 import scheduler disabled", "err", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
		day := ImportDate(time.Now())
		rows, err := g.ImportDay(ctx, day)
		if err != nil {
			logger.Error("ga4 import failed", "date", day.Format(dateLayout), "err", err)
			continue
		}
		logger.Info("ga4 import done", "date", day.Format(dateLayout), "rows", rows)
	}
}
