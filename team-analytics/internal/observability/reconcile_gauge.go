package observability

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
)

// ReconcileGauge holds ga4_reconcile_diff_ratio{event_type}.
type ReconcileGauge struct {
	mu sync.RWMutex
	v  map[string]float64
}

// NewReconcileGauge returns an empty gauge.
func NewReconcileGauge() *ReconcileGauge { return &ReconcileGauge{v: map[string]float64{}} }

// Set records the latest ratio for an event type.
func (g *ReconcileGauge) Set(eventType string, ratio float64) {
	g.mu.Lock()
	g.v[eventType] = ratio
	g.mu.Unlock()
}

// Get returns the ratio and whether it was ever set.
func (g *ReconcileGauge) Get(eventType string) (float64, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	v, ok := g.v[eventType]
	return v, ok
}

// WritePrometheus writes the Prometheus text exposition.
func (g *ReconcileGauge) WritePrometheus(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	keys := make([]string, 0, len(g.v))
	for k := range g.v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if _, err := fmt.Fprint(w, "# TYPE ga4_reconcile_diff_ratio gauge\n"); err != nil {
		return err
	}
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "ga4_reconcile_diff_ratio{event_type=%q} %g\n", k, g.v[k]); err != nil {
			return err
		}
	}
	return nil
}

// MetricsHandler serves the gauge as Prometheus text on GET /metrics.
func MetricsHandler(g *ReconcileGauge) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_ = g.WritePrometheus(w)
	})
	return mux
}
