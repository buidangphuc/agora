package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// DuckDB: counts, visitors, missing-listing ratio, lag (null ingested_at
// excluded), freshness, window bounds and counters.
func TestDuckDBTrackingQuality(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	rec := func(id, typ, listing, anon string, ago time.Duration) *warehouse.TrackingRecord {
		return &warehouse.TrackingRecord{EventID: id, EventType: typ, ListingID: listing, AnonymousID: anon, OccurredAt: now.Add(-ago)}
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		rec("v1", "view", "l1", "a1", 10*time.Minute),
		rec("v2", "view", "l1", "a1", 20*time.Minute),
		rec("v3", "view", "", "a2", 30*time.Minute),
		rec("v4", "view", "", "a3", 40*time.Minute),
		rec("p1", "purchase", "", "a1", 50*time.Minute), // not listing-scoped
		rec("old", "view", "l1", "a9", 3*time.Hour),     // outside a 1h window
		rec("legacy", "view", "l1", "a8", 5*time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	// Set deterministic ingested_at: lag 10s for 5 rows, 100s for one, null for legacy.
	set := func(id string, lagSeconds int) {
		if _, err := w.DB().Exec(
			"UPDATE tracking_events SET ingested_at = occurred_at + to_seconds(?) WHERE event_id = ?", lagSeconds, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"v1", "v2", "v3", "v4", "p1"} {
		set(id, 10)
	}
	set("old", 1000) // outside the window: must not move the percentiles
	if _, err := w.DB().Exec("UPDATE tracking_events SET ingested_at = NULL WHERE event_id = 'legacy'"); err != nil {
		t.Fatal(err)
	}
	set("v1", 100)
	if _, err := w.DB().Exec("INSERT INTO tracking_ingest_counters VALUES (?, 2, 5), (?, 1, 1)",
		now.Add(-30*time.Minute).Truncate(time.Hour), now.Add(-5*time.Hour).Truncate(time.Hour)); err != nil {
		t.Fatal(err)
	}

	repo := query.NewDuckDBRepository(w.DB())
	got, err := repo.TrackingQuality(ctx, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}

	byType := map[string]query.TypeQuality{}
	for _, ty := range got.Types {
		byType[ty.EventType] = ty
	}
	view := byType["view"]
	if view.Events != 5 || view.Visitors != 4 || !view.ListingScoped {
		t.Fatalf("view = %+v, want 5 events, 4 visitors, scoped (old row excluded)", view)
	}
	if view.MissingListingRatio < 0.399 || view.MissingListingRatio > 0.401 {
		t.Fatalf("view missing ratio = %v, want 0.4", view.MissingListingRatio)
	}
	if p := byType["purchase"]; p.Events != 1 || p.ListingScoped || p.MissingListingRatio != 0 {
		t.Fatalf("purchase = %+v, want 1 event, not scoped, ratio 0", p)
	}
	// Lag over {10,10,10,10,100}: legacy (null) and old (outside) are excluded.
	if !got.HasLag || got.LagP50Seconds != 10 || got.LagP95Seconds < 81.9 || got.LagP95Seconds > 82.1 {
		t.Fatalf("lag = %v/%v has=%v, want 10/82", got.LagP50Seconds, got.LagP95Seconds, got.HasLag)
	}
	if want := now.Add(-10*time.Minute + 100*time.Second); !got.LastIngestedAt.Equal(want) {
		t.Fatalf("last ingested = %v, want %v", got.LastIngestedAt, want)
	}
	if got.DecodeFailures != 2 || got.Duplicates != 5 {
		t.Fatalf("counters = %d/%d, want 2/5 (hour 5h ago excluded)", got.DecodeFailures, got.Duplicates)
	}

	// A wider window pulls in the old row and the older counter hour.
	wide, err := repo.TrackingQuality(ctx, now.Add(-6*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if wide.DecodeFailures != 3 || wide.Duplicates != 6 {
		t.Fatalf("wide counters = %d/%d, want 3/6", wide.DecodeFailures, wide.Duplicates)
	}

	// An empty window has no types, no lag and no freshness.
	empty, err := repo.TrackingQuality(ctx, now.Add(24*time.Hour), now.Add(25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Types) != 0 || empty.HasLag || !empty.LastIngestedAt.IsZero() {
		t.Fatalf("empty window = %+v", empty)
	}
}

// stubQuality serves canned data so every status reason is tested in isolation.
type stubQuality struct {
	query.Repository
	data  query.TrackingQualityData
	err   error
	since time.Time
	until time.Time
}

func (s *stubQuality) TrackingQuality(_ context.Context, since, until time.Time) (query.TrackingQualityData, error) {
	s.since, s.until = since, until
	return s.data, s.err
}

func serviceAt(now time.Time, stub *stubQuality) *query.Service {
	return query.NewService(stub, query.WithTrackingThresholds(query.TrackingThresholds{
		StaleAfter: 900 * time.Second, LagP95Max: 300 * time.Second, MissingListingMaxRatio: 0.05,
	}), query.WithClock(func() time.Time { return now }))
}

func TestGetTrackingQualityReport_Status(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Minute)
	good := []query.TypeQuality{{EventType: "view", Events: 10, Visitors: 3, ListingScoped: true}}
	cases := []struct {
		name    string
		data    query.TrackingQualityData
		status  string
		reasons []string
	}{
		{"ok", query.TrackingQualityData{Types: good, HasLag: true, LagP95Seconds: 300, LastIngestedAt: fresh}, "OK", nil},
		{"stale: last ingest too old", query.TrackingQualityData{Types: good, LastIngestedAt: now.Add(-901 * time.Second)}, "DEGRADED", []string{"stale"}},
		{"not stale at exactly the threshold", query.TrackingQualityData{Types: good, LastIngestedAt: now.Add(-900 * time.Second)}, "OK", nil},
		{"stale: no event in the window", query.TrackingQualityData{}, "DEGRADED", []string{"stale"}},
		{"lagging", query.TrackingQualityData{Types: good, HasLag: true, LagP95Seconds: 300.5, LastIngestedAt: fresh}, "DEGRADED", []string{"lagging"}},
		{"incomplete", query.TrackingQualityData{
			Types:          []query.TypeQuality{{EventType: "view", Events: 10, MissingListingRatio: 0.06, ListingScoped: true}},
			LastIngestedAt: fresh}, "DEGRADED", []string{"incomplete"}},
		{"ratio at the threshold is not incomplete", query.TrackingQualityData{
			Types:          []query.TypeQuality{{EventType: "view", Events: 10, MissingListingRatio: 0.05, ListingScoped: true}},
			LastIngestedAt: fresh}, "OK", nil},
		{"unscoped ratio is ignored", query.TrackingQualityData{
			Types:          []query.TypeQuality{{EventType: "purchase", Events: 10, MissingListingRatio: 1}},
			LastIngestedAt: fresh}, "OK", nil},
		{"all reasons are sorted", query.TrackingQualityData{
			Types:         []query.TypeQuality{{EventType: "view", Events: 10, MissingListingRatio: 0.5, ListingScoped: true}},
			HasLag:        true,
			LagP95Seconds: 999, LastIngestedAt: now.Add(-time.Hour)}, "DEGRADED", []string{"incomplete", "lagging", "stale"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, err := serviceAt(now, &stubQuality{data: c.data}).GetTrackingQualityReport(
				asPrincipal("admin-1", "user", "admin"), &analyticsv1.GetTrackingQualityReportRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if resp.GetStatus() != c.status {
				t.Fatalf("status = %q, want %q (reasons %v)", resp.GetStatus(), c.status, resp.GetReasons())
			}
			if len(resp.GetReasons()) != len(c.reasons) {
				t.Fatalf("reasons = %v, want %v", resp.GetReasons(), c.reasons)
			}
			for i := range c.reasons {
				if resp.GetReasons()[i] != c.reasons[i] {
					t.Fatalf("reasons = %v, want %v", resp.GetReasons(), c.reasons)
				}
			}
		})
	}
}

func TestGetTrackingQualityReport_Window(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	admin := asPrincipal("admin-1", "user", "admin")
	cases := []struct {
		hours     uint32
		wantCode  codes.Code
		wantHours uint32
	}{
		{0, codes.OK, 24}, {1, codes.OK, 1}, {168, codes.OK, 168}, {169, codes.InvalidArgument, 0}, {500, codes.InvalidArgument, 0},
	}
	for _, c := range cases {
		stub := &stubQuality{}
		resp, err := serviceAt(now, stub).GetTrackingQualityReport(admin, &analyticsv1.GetTrackingQualityReportRequest{WindowHours: c.hours})
		if status.Code(err) != c.wantCode {
			t.Fatalf("hours %d: code = %v, want %v", c.hours, status.Code(err), c.wantCode)
		}
		if c.wantCode != codes.OK {
			continue
		}
		if resp.GetWindowHours() != c.wantHours {
			t.Fatalf("hours %d: window_hours = %d, want %d", c.hours, resp.GetWindowHours(), c.wantHours)
		}
		if !stub.until.Equal(now) || !stub.since.Equal(now.Add(-time.Duration(c.wantHours)*time.Hour)) {
			t.Fatalf("hours %d: queried [%v, %v]", c.hours, stub.since, stub.until)
		}
	}
}

func TestGetTrackingQualityReport_Access(t *testing.T) {
	now := time.Now()
	svc := serviceAt(now, &stubQuality{})
	req := &analyticsv1.GetTrackingQualityReportRequest{}
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"no principal", context.Background(), codes.Unauthenticated},
		{"anonymous principal (no admin scope)", asPrincipal("anonymous", "anonymous", "catalog.read"), codes.PermissionDenied},
		{"buyer", asPrincipal("buyer-1", "user", "catalog.read"), codes.PermissionDenied},
		{"service without admin", asPrincipal("svc-x", "service", "order.read"), codes.PermissionDenied},
		{"admin", asPrincipal("admin-1", "user", "admin"), codes.OK},
	}
	for _, c := range cases {
		if _, err := svc.GetTrackingQualityReport(c.ctx, req); status.Code(err) != c.want {
			t.Errorf("%s: code = %v, want %v", c.name, status.Code(err), c.want)
		}
	}
}

func TestGetTrackingQualityReport_Errors(t *testing.T) {
	admin := asPrincipal("admin-1", "user", "admin")
	req := &analyticsv1.GetTrackingQualityReportRequest{}
	if _, err := serviceAt(time.Now(), &stubQuality{err: errors.New("boom")}).GetTrackingQualityReport(admin, req); status.Code(err) != codes.Internal {
		t.Errorf("repo error: code = %v, want Internal", status.Code(err))
	}
	// A repository that cannot measure quality (e.g. the in-memory one).
	if _, err := query.NewService(query.NewMemoryRepository()).GetTrackingQualityReport(admin, req); status.Code(err) != codes.Unavailable {
		t.Errorf("unsupported repo: code = %v, want Unavailable", status.Code(err))
	}
}
