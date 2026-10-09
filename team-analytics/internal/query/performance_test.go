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

func perfRepo(t *testing.T, recs []*warehouse.TrackingRecord) *query.DuckDBRepository {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	if err := w.Write(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	return query.NewDuckDBRepository(w.DB())
}

func TestDuckDBRecommendationPerformance(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ev := func(id, typ, listing, user, imp, placement, model string, ago time.Duration) *warehouse.TrackingRecord {
		return &warehouse.TrackingRecord{
			EventID: id, EventType: typ, ListingID: listing, AnonymousID: "anon-" + user,
			PrincipalID: user, PrincipalType: "user", ImpressionID: imp, PlacementID: placement,
			ModelVersion: model, OccurredAt: now.Add(-ago),
		}
	}
	h := time.Hour
	repo := perfRepo(t, []*warehouse.TrackingRecord{
		// home_feed / M1: impression I1 (2 items), I2 (1 item); two clicks on I1.
		ev("i1a", "impression", "l1", "u1", "I1", "home_feed", "M1", 5*h),
		ev("i1b", "impression", "l2", "u1", "I1", "home_feed", "M1", 5*h),
		ev("i2", "impression", "l1", "u2", "I2", "home_feed", "M1", 5*h),
		ev("c1", "click", "l1", "u1", "I1", "", "", 4*h),
		ev("c1b", "click", "l1", "u1", "I1", "", "", 3*h), // later click, same listing: not the earliest
		ev("cart1", "add_to_cart", "l1", "u1", "", "", "", 2*h),
		ev("buy1", "purchase", "l1", "u1", "", "", "", 90*time.Minute),
		// purchase of a listing never clicked from a recommendation
		ev("buy-other", "purchase", "l9", "u1", "", "", "", 2*h),
		// click with no impression id, and click with an unknown impression id
		ev("c-none", "click", "l1", "u3", "", "", "", 4*h),
		ev("c-unk", "click", "l1", "u3", "Ix", "", "", 4*h),
		ev("buy-u3", "purchase", "l1", "u3", "", "", "", 3*h),
		// serving-fallback placement, 1 impression each.
		ev("f1", "impression", "l3", "u4", "F1", "cart_cross_sell", "serving-fallback", 2*h),
		ev("f2", "impression", "l3", "u4", "F2", "cart_cross_sell", "M2", 2*h),
		// impression outside the window is ignored
		ev("old", "impression", "l1", "u5", "Iold", "home_feed", "M1", 50*h),
		ev("oldc", "click", "l1", "u5", "Iold", "", "", 49*h),
	})

	got, err := repo.RecommendationPerformance(context.Background(), now.Add(-24*h), now, 24)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]query.PerformanceRow{}
	for _, r := range got {
		by[r.PlacementID+"/"+r.ModelVersion] = r
	}
	if len(got) != 3 {
		t.Fatalf("rows = %+v, want 3", got)
	}
	m1 := by["home_feed/M1"]
	want := query.PerformanceRow{PlacementID: "home_feed", ModelVersion: "M1", Impressions: 2, ItemImpressions: 3, Clicks: 2, AddToCarts: 1, Purchases: 1}
	if m1 != want {
		t.Fatalf("home_feed/M1 = %+v, want %+v (purchase credited once, unclicked and unattributed purchases excluded)", m1, want)
	}
	if f := by["cart_cross_sell/serving-fallback"]; f.Impressions != 1 || f.Clicks != 0 || f.Purchases != 0 {
		t.Fatalf("fallback row = %+v", f)
	}
	if _, ok := by["cart_cross_sell/M2"]; !ok {
		t.Fatalf("real model row missing: %+v", got)
	}

	// A 1-hour attribution window drops the purchase (1.5h after the later
	// click) but keeps the cart (1h after it).
	narrow, err := repo.RecommendationPerformance(context.Background(), now.Add(-24*h), now, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range narrow {
		if r.PlacementID == "home_feed" && (r.AddToCarts != 1 || r.Purchases != 0 || r.Clicks != 2) {
			t.Fatalf("narrow attribution = %+v, want 1 cart, 0 purchases, 2 clicks", r)
		}
	}

	// A wide window pulls in the old impression and its click.
	wide, err := repo.RecommendationPerformance(context.Background(), now.Add(-72*h), now, 24)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range wide {
		if r.PlacementID == "home_feed" && (r.Impressions != 3 || r.Clicks != 3) {
			t.Fatalf("wide = %+v, want 3 impressions, 3 clicks", r)
		}
	}

	// An empty window has no rows.
	empty, err := repo.RecommendationPerformance(context.Background(), now.Add(24*h), now.Add(25*h), 24)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty window = %+v, %v", empty, err)
	}
}

// A purchase after the attribution window is not credited; a purchase inside is.
func TestDuckDBRecommendationPerformance_OutsideAttributionWindow(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ev := func(id, typ, imp, placement, model string, ago time.Duration) *warehouse.TrackingRecord {
		return &warehouse.TrackingRecord{EventID: id, EventType: typ, ListingID: "l1", AnonymousID: "a", PrincipalID: "u", PrincipalType: "user",
			ImpressionID: imp, PlacementID: placement, ModelVersion: model, OccurredAt: now.Add(-ago)}
	}
	repo := perfRepo(t, []*warehouse.TrackingRecord{
		ev("i", "impression", "I", "p", "M", 30*time.Hour),
		ev("c", "click", "I", "", "", 29*time.Hour),
		ev("in", "purchase", "", "", "", 20*time.Hour), // 9h after click
		ev("out", "purchase", "", "", "", 1*time.Hour), // 28h after click
	})
	got, err := repo.RecommendationPerformance(context.Background(), now.Add(-48*time.Hour), now, 24)
	if err != nil || len(got) != 1 || got[0].Purchases != 1 {
		t.Fatalf("got %+v, %v; want exactly the in-window purchase", got, err)
	}
}

type stubPerf struct {
	query.Repository
	rows  []query.PerformanceRow
	err   error
	since time.Time
	until time.Time
	hours int
}

func (s *stubPerf) RecommendationPerformance(_ context.Context, since, until time.Time, hours int) ([]query.PerformanceRow, error) {
	s.since, s.until, s.hours = since, until, hours
	return s.rows, s.err
}

func TestGetRecommendationPerformance(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	admin := asPrincipal("admin-1", "user", "admin")
	stub := &stubPerf{rows: []query.PerformanceRow{
		{PlacementID: "cart", ModelVersion: "M", Impressions: 3, ItemImpressions: 10, Clicks: 2, AddToCarts: 1, Purchases: 1},
		{PlacementID: "cart", ModelVersion: "serving-fallback", Impressions: 1, ItemImpressions: 4},
		{PlacementID: "home", ModelVersion: "M", Impressions: 2, ItemImpressions: 2},
	}}
	svc := query.NewService(stub, query.WithClock(func() time.Time { return now }), query.WithAttributionWindowHours(12))
	resp, err := svc.GetRecommendationPerformance(admin, &analyticsv1.GetRecommendationPerformanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.WindowHours != 24 || resp.AttributionWindowHours != 12 || stub.hours != 12 || !stub.since.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("window = %d/%d, stub = %+v", resp.WindowHours, resp.AttributionWindowHours, stub)
	}
	r := resp.Rows[0]
	if r.Ctr != 0.2 || r.ConversionRate != 0.5 {
		t.Fatalf("ctr/conversion = %v/%v, want 0.2/0.5", r.Ctr, r.ConversionRate)
	}
	if r := resp.Rows[1]; r.Ctr != 0 || r.ConversionRate != 0 {
		t.Fatalf("no clicks: ctr/conversion = %v/%v, want 0/0", r.Ctr, r.ConversionRate)
	}
	if len(resp.Fallback) != 2 || resp.Fallback[0].PlacementId != "cart" || resp.Fallback[0].FallbackShare != 0.25 ||
		resp.Fallback[1].PlacementId != "home" || resp.Fallback[1].FallbackShare != 0 {
		t.Fatalf("fallback = %+v", resp.Fallback)
	}
}

func TestGetRecommendationPerformance_Bounds(t *testing.T) {
	admin := asPrincipal("admin-1", "user", "admin")
	svc := query.NewService(&stubPerf{})
	for hours, want := range map[uint32]codes.Code{0: codes.OK, 1: codes.OK, 168: codes.OK, 169: codes.InvalidArgument, 100000: codes.InvalidArgument} {
		_, err := svc.GetRecommendationPerformance(admin, &analyticsv1.GetRecommendationPerformanceRequest{WindowHours: hours})
		if status.Code(err) != want {
			t.Errorf("window %d: code = %v, want %v", hours, status.Code(err), want)
		}
	}
}

func TestGetRecommendationPerformance_AccessAndErrors(t *testing.T) {
	req := &analyticsv1.GetRecommendationPerformanceRequest{}
	svc := query.NewService(&stubPerf{})
	for name, c := range map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"no principal":          {context.Background(), codes.Unauthenticated},
		"anonymous":             {asPrincipal("anonymous", "anonymous", "catalog.read"), codes.PermissionDenied},
		"buyer":                 {asPrincipal("b-1", "user", "buyer"), codes.PermissionDenied},
		"service without admin": {asPrincipal("svc-x", "service", "order.read"), codes.PermissionDenied},
		"admin":                 {asPrincipal("admin-1", "user", "admin"), codes.OK},
	} {
		if _, err := svc.GetRecommendationPerformance(c.ctx, req); status.Code(err) != c.want {
			t.Errorf("%s: code = %v, want %v", name, status.Code(err), c.want)
		}
	}
	admin := asPrincipal("admin-1", "user", "admin")
	if _, err := query.NewService(&stubPerf{err: errors.New("boom")}).GetRecommendationPerformance(admin, req); status.Code(err) != codes.Internal {
		t.Errorf("repo error: code = %v, want Internal", status.Code(err))
	}
	// A repository without the optional interface is Unavailable.
	if _, err := query.NewService(newRepo()).GetRecommendationPerformance(admin, req); status.Code(err) != codes.Unavailable {
		t.Errorf("no interface: code = %v, want Unavailable", status.Code(err))
	}
}
