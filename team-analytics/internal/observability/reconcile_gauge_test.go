package observability_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/buidangphuc/team-analytics/internal/observability"
)

// Scenario: Divergence beyond the threshold raises the alert (gauge ga4_reconcile_diff_ratio{event_type}).
func TestReconcileGauge_ExposesRatioPerEventType(t *testing.T) {
	g := observability.NewReconcileGauge()
	if _, ok := g.Get("view"); ok {
		t.Fatal("unset gauge must report not-set")
	}
	g.Set("view", 0.5)
	g.Set("click", 3)
	g.Set("view", 0.25) // last write wins
	if v, ok := g.Get("view"); !ok || v != 0.25 {
		t.Fatalf("view = %v/%v, want 0.25", v, ok)
	}
	var b bytes.Buffer
	if err := g.WritePrometheus(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`ga4_reconcile_diff_ratio{event_type="view"} 0.25`, `ga4_reconcile_diff_ratio{event_type="click"} 3`} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q in:\n%s", want, out)
		}
	}
}
