package rpc_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/observability"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// Seams under test (add-ga4-export-import / reconcile):
//   query.GA4ReconcileRepository (optional interface on the repository):
//     GA4Reconcile(ctx, day time.Time) (query.GA4Reconcile, error)
//   query.GA4Reconcile{ImportRun bool; Types []query.GA4TypeCount{EventType string; GA4, FirstParty int64}}
//   rpc.WithGA4Reconcile(maxDiffRatio float64, gauge rpc.RatioGauge) service option;
//   rpc.RatioGauge = interface{ Set(eventType string, ratio float64) }
//   observability.NewReconcileGauge(): Set / Get / WritePrometheus (metric ga4_reconcile_diff_ratio{event_type}).
// The reconciled day is yesterday (UTC) relative to the service clock.

var rcNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

type stubReconcile struct {
	stubQuality
	rec    query.GA4Reconcile
	err    error
	gotDay time.Time
	called bool
}

func (s *stubReconcile) GA4Reconcile(_ context.Context, day time.Time) (query.GA4Reconcile, error) {
	s.called, s.gotDay = true, day
	return s.rec, s.err
}

func rcHealthy() stubQuality {
	return stubQuality{data: query.TrackingQualityData{
		Types:          []query.TypeQuality{{EventType: "view", Events: 100, Visitors: 10, ListingScoped: true}},
		LastIngestedAt: rcNow.Add(-time.Minute),
	}}
}

func rcService(stub *stubReconcile, g rpc.RatioGauge, max float64) *rpc.Handler {
	return mustHandler(stub,
		rpc.WithTrackingThresholds(rpc.TrackingThresholds{StaleAfter: 900 * time.Second, LagP95Max: 300 * time.Second, MissingListingMaxRatio: 0.05}),
		rpc.WithGA4Reconcile(max, g),
		rpc.WithClock(func() time.Time { return rcNow }))
}

func rcReport(t *testing.T, stub *stubReconcile, g rpc.RatioGauge, max float64) *analyticsv1.GetTrackingQualityReportResponse {
	t.Helper()
	resp, err := rcService(stub, g, max).GetTrackingQualityReport(asPrincipal("admin-1", "user", "admin"), &analyticsv1.GetTrackingQualityReportRequest{})
	if err != nil {
		t.Fatalf("GetTrackingQualityReport: %v", err)
	}
	return resp
}

func has(rs []string, r string) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}

func rcOne(typ string, ga4, fp int64) query.GA4Reconcile {
	return query.GA4Reconcile{ImportRun: true, Types: []query.GA4TypeCount{{EventType: typ, GA4: ga4, FirstParty: fp}}}
}

// Scenario: Divergence beyond the threshold raises the alert
func TestReconcile_DivergenceBeyondThresholdRaisesTheAlert(t *testing.T) {
	g := observability.NewReconcileGauge()
	stub := &stubReconcile{stubQuality: rcHealthy(), rec: rcOne("view", 50, 100)}
	resp := rcReport(t, stub, g, 0.35)
	if resp.Status != "DEGRADED" || !has(resp.Reasons, "ga4_divergence") {
		t.Fatalf("status=%s reasons=%v, want DEGRADED with ga4_divergence", resp.Status, resp.Reasons)
	}
	if v, ok := g.Get("view"); !ok || math.Abs(v-0.5) > 1e-9 {
		t.Fatalf("gauge view = %v (set=%v), want 0.5", v, ok)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !stub.gotDay.Equal(want) {
		t.Fatalf("reconciled day = %v, want yesterday UTC %v", stub.gotDay, want)
	}
}

// Scenario: Divergence at the threshold does not alert
func TestReconcile_DivergenceAtTheThresholdDoesNotAlert(t *testing.T) {
	g := observability.NewReconcileGauge()
	resp := rcReport(t, &stubReconcile{stubQuality: rcHealthy(), rec: rcOne("view", 65, 100)}, g, 0.35)
	if has(resp.Reasons, "ga4_divergence") || resp.Status != "OK" {
		t.Fatalf("status=%s reasons=%v, want OK without ga4_divergence", resp.Status, resp.Reasons)
	}
}

// Scenario: Zero first-party events are handled
func TestReconcile_ZeroFirstPartyEventsAreHandled(t *testing.T) {
	g := observability.NewReconcileGauge()
	resp := rcReport(t, &stubReconcile{stubQuality: rcHealthy(), rec: rcOne("click", 3, 0)}, g, 0.35)
	if !has(resp.Reasons, "ga4_divergence") {
		t.Fatalf("reasons=%v, want ga4_divergence", resp.Reasons)
	}
	if v, ok := g.Get("click"); !ok || v != 3 || math.IsInf(v, 0) || math.IsNaN(v) {
		t.Fatalf("gauge click = %v (set=%v), want 3", v, ok)
	}
}

// Scenario: A missing import is reported, not treated as divergence
func TestReconcile_MissingImportIsReportedNotDivergence(t *testing.T) {
	g := observability.NewReconcileGauge()
	// Counts present but no run: must not be compared.
	rec := query.GA4Reconcile{ImportRun: false, Types: []query.GA4TypeCount{{EventType: "view", GA4: 0, FirstParty: 100}}}
	resp := rcReport(t, &stubReconcile{stubQuality: rcHealthy(), rec: rec}, g, 0.35)
	if !has(resp.Reasons, "ga4_missing") || has(resp.Reasons, "ga4_divergence") {
		t.Fatalf("reasons=%v, want ga4_missing and no ga4_divergence", resp.Reasons)
	}
	if _, ok := g.Get("view"); ok {
		t.Fatal("gauge must not carry a divergence value for a day without an import run")
	}
}

// Scenario: A missing import adds a reason only
func TestReconcile_MissingImportAddsAReasonOnly(t *testing.T) {
	resp := rcReport(t, &stubReconcile{stubQuality: rcHealthy(), rec: query.GA4Reconcile{ImportRun: false}}, observability.NewReconcileGauge(), 0.35)
	if resp.Status != "OK" || !has(resp.Reasons, "ga4_missing") {
		t.Fatalf("status=%s reasons=%v, want OK with ga4_missing", resp.Status, resp.Reasons)
	}
}

// Scenario: Reconciliation failure does not break the report
func TestReconcile_FailureDoesNotBreakTheReport(t *testing.T) {
	g := observability.NewReconcileGauge()
	stub := &stubReconcile{stubQuality: rcHealthy(), err: errors.New("ga4_events_daily unreadable")}
	resp := rcReport(t, stub, g, 0.35)
	if !stub.called {
		t.Fatal("reconcile read was not attempted")
	}
	if resp.Status != "OK" || len(resp.Reasons) != 0 || len(resp.Types) != 1 || resp.Types[0].Events != 100 {
		t.Fatalf("report = status %s reasons %v types %v, want the unchanged existing report", resp.Status, resp.Reasons, resp.Types)
	}
}

// Scenario: Only admins see reconciliation reasons (service-level: the admin gate runs before any GA4 read)
func TestReconcile_OnlyAdminsSeeReconciliationReasons(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"anonymous": asPrincipal("anonymous", "anonymous", "catalog.read"),
		"buyer":     asPrincipal("u-1", "user", "catalog.read"),
	} {
		want := codes.PermissionDenied
		if name == "anonymous" {
			want = codes.Unauthenticated
		}
		stub := &stubReconcile{stubQuality: rcHealthy(), rec: rcOne("view", 50, 100)}
		_, err := rcService(stub, observability.NewReconcileGauge(), 0.35).GetTrackingQualityReport(ctx, &analyticsv1.GetTrackingQualityReportRequest{})
		if status.Code(err) != want {
			t.Errorf("%s: code = %v, want %v", name, status.Code(err), want)
		}
		if stub.called {
			t.Errorf("%s: GA4 data was read before the admin gate", name)
		}
	}
}

// Scenario: Divergence beyond the threshold raises the alert (DuckDB read: GA4 names mapped to event types,
// purchase excluded, same UTC day, run ledger decides missing).
func TestReconcile_DuckDBCountsSameUTCDayAndExcludesPurchase(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var recs []*warehouse.TrackingRecord
	add := func(typ string, n int, at time.Time) {
		for i := 0; i < n; i++ {
			recs = append(recs, &warehouse.TrackingRecord{EventID: typ + at.Format("150405") + string(rune('a'+i)) + string(rune('A'+n)), EventType: typ, AnonymousID: "a", OccurredAt: at})
		}
	}
	add("view", 4, day.Add(23*time.Hour+59*time.Minute))
	add("view", 2, day.Add(-time.Minute)) // previous UTC day: excluded
	add("add_to_cart", 2, day.Add(time.Hour))
	add("purchase", 5, day.Add(time.Hour))
	if err := w.Write(ctx, recs); err != nil {
		t.Fatal(err)
	}
	db := w.DB()
	for _, s := range []string{
		"INSERT INTO ga4_events_daily VALUES (DATE '2026-10-01','view_item',3),(DATE '2026-10-01','add_to_cart',1),(DATE '2026-10-01','purchase',9)",
		"INSERT INTO ga4_import_runs VALUES (DATE '2026-10-01', 4, TIMESTAMP '2026-10-02 01:00:00')",
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	repo := query.NewDuckDBRepository(db)
	rr, ok := any(repo).(query.GA4ReconcileRepository)
	if !ok {
		t.Fatal("DuckDBRepository must implement query.GA4ReconcileRepository")
	}
	got, err := rr.GA4Reconcile(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ImportRun {
		t.Fatal("ImportRun = false, want true (run ledger has the day)")
	}
	by := map[string]query.GA4TypeCount{}
	for _, c := range got.Types {
		by[c.EventType] = c
	}
	if c := by["view"]; c.GA4 != 3 || c.FirstParty != 4 {
		t.Fatalf("view = %+v, want ga4 3 first-party 4", c)
	}
	if c := by["add_to_cart"]; c.GA4 != 1 || c.FirstParty != 2 {
		t.Fatalf("add_to_cart = %+v, want ga4 1 first-party 2", c)
	}
	if _, found := by["purchase"]; found {
		t.Fatal("purchase must be excluded from reconciliation")
	}
	none, err := rr.GA4Reconcile(ctx, day.AddDate(0, 0, 1))
	if err != nil || none.ImportRun {
		t.Fatalf("day without run: %+v err=%v, want ImportRun=false", none, err)
	}
}
