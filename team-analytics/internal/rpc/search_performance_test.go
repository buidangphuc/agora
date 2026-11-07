package rpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

type searchStub struct {
	stubPerf
	searchRows  []query.PerformanceRow
	searchErr   error
	searchCalls int
	sSince      time.Time
	sUntil      time.Time
	sHours      int
}

func (s *searchStub) SearchPerformance(_ context.Context, since, until time.Time, hours int) ([]query.PerformanceRow, error) {
	s.searchCalls++
	s.sSince, s.sUntil, s.sHours = since, until, hours
	return s.searchRows, s.searchErr
}

func (s *stubPerf) SearchPerformance(_ context.Context, _, _ time.Time, _ int) ([]query.PerformanceRow, error) {
	return nil, nil
}

func TestGetSearchPerformance_AdminOnlyGateRunsBeforeTheRepository(t *testing.T) {
	req := &analyticsv1.GetSearchPerformanceRequest{}
	for name, c := range map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"no principal":          {context.Background(), codes.Unauthenticated},
		"anonymous":             {asPrincipal("anonymous", "anonymous", "catalog.read"), codes.Unauthenticated},
		"buyer":                 {asPrincipal("b-1", "user", "buyer"), codes.PermissionDenied},
		"seller":                {asPrincipal("s-1", "user", "seller"), codes.PermissionDenied},
		"service without admin": {asPrincipal("svc-x", "service", "order.read"), codes.PermissionDenied},
		"admin":                 {asPrincipal("admin-1", "user", "admin"), codes.OK},
	} {
		stub := &searchStub{}
		_, err := mustHandler(stub).GetSearchPerformance(c.ctx, req)
		if status.Code(err) != c.want {
			t.Errorf("%s: code = %v, want %v", name, status.Code(err), c.want)
		}
		if c.want != codes.OK && stub.searchCalls != 0 {
			t.Errorf("%s: repository was queried %d times before the admin gate", name, stub.searchCalls)
		}
	}
}

func TestGetSearchPerformance_GateBeforeWindowValidation(t *testing.T) {
	buyer := asPrincipal("b-1", "user", "buyer")
	_, err := mustHandler(&searchStub{}).GetSearchPerformance(buyer, &analyticsv1.GetSearchPerformanceRequest{WindowHours: 169})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied before window validation", status.Code(err))
	}
	_, err = mustHandler(&searchStub{}).GetSearchPerformance(context.Background(), &analyticsv1.GetSearchPerformanceRequest{WindowHours: 169})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated before window validation", status.Code(err))
	}
}

func TestGetSearchPerformance_ZeroWindowResolvesTo24(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	admin := asPrincipal("admin-1", "user", "admin")
	stub := &searchStub{}
	svc := mustHandler(stub, rpc.WithClock(func() time.Time { return now }), rpc.WithAttributionWindowHours(12))
	resp, err := svc.GetSearchPerformance(admin, &analyticsv1.GetSearchPerformanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.WindowHours != 24 {
		t.Fatalf("window_hours = %d, want 24 (not the 168 default of GetRecommendationPerformance)", resp.WindowHours)
	}
	if !stub.sSince.Equal(now.Add(-24*time.Hour)) || !stub.sUntil.Equal(now) || stub.sHours != 12 {
		t.Fatalf("repository saw since=%v until=%v attribution=%d, want now-24h, now, 12", stub.sSince, stub.sUntil, stub.sHours)
	}
	if len(resp.Rows) != 0 {
		t.Fatalf("rows = %+v, want none and no error for an empty window", resp.Rows)
	}
}

func TestGetSearchPerformance_WindowBounds(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	admin := asPrincipal("admin-1", "user", "admin")
	for hours, want := range map[uint32]codes.Code{0: codes.OK, 1: codes.OK, 24: codes.OK, 168: codes.OK, 169: codes.InvalidArgument, 100000: codes.InvalidArgument} {
		stub := &searchStub{}
		resp, err := mustHandler(stub, rpc.WithClock(func() time.Time { return now })).GetSearchPerformance(admin, &analyticsv1.GetSearchPerformanceRequest{WindowHours: hours})
		if status.Code(err) != want {
			t.Errorf("window %d: code = %v, want %v", hours, status.Code(err), want)
		}
		if want == codes.InvalidArgument && stub.searchCalls != 0 {
			t.Errorf("window %d: repository queried despite rejection", hours)
		}
		if want == codes.OK && hours > 0 && resp != nil && resp.WindowHours != hours {
			t.Errorf("window %d: response window_hours = %d", hours, resp.WindowHours)
		}
	}
}

func TestGetSearchPerformanceRejectsWindowOver168(t *testing.T) {
	admin := asPrincipal("admin-1", "user", "admin")
	stub := &searchStub{}
	_, err := mustHandler(stub).GetSearchPerformance(admin, &analyticsv1.GetSearchPerformanceRequest{WindowHours: 169})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if stub.searchCalls != 0 {
		t.Fatalf("repository queried %d times for a rejected window", stub.searchCalls)
	}
}

func TestGetSearchPerformance_RatiosAndAttributionWindow(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	admin := asPrincipal("admin-1", "user", "admin")
	stub := &searchStub{searchRows: []query.PerformanceRow{
		{PlacementID: "search_results", ModelVersion: "hybrid@v1", Impressions: 1, ItemImpressions: 10, Clicks: 4, AddToCarts: 1, Purchases: 2, MatureClicks: 4, MaturePurchases: 2},
		{PlacementID: "search_results", ModelVersion: "lexical@v1", Impressions: 1, ItemImpressions: 10, Clicks: 1},
		{PlacementID: "search_results", ModelVersion: "quiet", Impressions: 1, ItemImpressions: 5},
		{PlacementID: "search_results", ModelVersion: "open", Impressions: 1, ItemImpressions: 10, Clicks: 4, Purchases: 3, MatureClicks: 2, MaturePurchases: 1},
	}}
	svc := mustHandler(stub, rpc.WithClock(func() time.Time { return now }), rpc.WithAttributionWindowHours(12))
	resp, err := svc.GetSearchPerformance(admin, &analyticsv1.GetSearchPerformanceRequest{WindowHours: 48})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := svc.GetRecommendationPerformance(admin, &analyticsv1.GetRecommendationPerformanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.AttributionWindowHours != rec.AttributionWindowHours || resp.AttributionWindowHours != 12 {
		t.Fatalf("attribution_window_hours = %d, recommendation reports %d, want both 12", resp.AttributionWindowHours, rec.AttributionWindowHours)
	}
	if len(resp.Rows) != 4 {
		t.Fatalf("rows = %+v", resp.Rows)
	}
	by := map[string]*analyticsv1.RecommendationPerformanceRow{}
	for _, r := range resp.Rows {
		by[r.ModelVersion] = r
	}
	hy := by["hybrid@v1"]
	if hy.PlacementId != "search_results" || hy.Impressions != 1 || hy.ItemImpressions != 10 || hy.Clicks != 4 || hy.AddToCarts != 1 || hy.Purchases != 2 || hy.Ctr != 0.4 || hy.ConversionRate != 0.5 {
		t.Fatalf("hybrid@v1 = %+v, want placement search_results, clicks 4, purchases 2, ctr 0.4, conversion_rate 0.5", hy)
	}
	if lx := by["lexical@v1"]; lx.Ctr != 0.1 || lx.ConversionRate != 0 {
		t.Fatalf("lexical@v1 ctr/conversion = %v/%v, want 0.1/0", lx.Ctr, lx.ConversionRate)
	}
	if q := by["quiet"]; q.Ctr != 0 || q.ConversionRate != 0 {
		t.Fatalf("no clicks: ctr/conversion = %v/%v, want 0/0", q.Ctr, q.ConversionRate)
	}
	if o := by["open"]; o.ConversionRate != 0.5 {
		t.Fatalf("conversion_rate = %v, want purchases over mature clicks only (0.5)", o.ConversionRate)
	}
}

func TestGetSearchPerformanceRowsArePlacementSearchResults(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	h := time.Hour
	tracking := func(id, typ, listing, imp, placement, model string, ago time.Duration) *warehouse.TrackingRecord {
		return &warehouse.TrackingRecord{
			EventID: id, EventType: typ, ListingID: listing, AnonymousID: "a", PrincipalID: "u",
			PrincipalType: "user", ImpressionID: imp, PlacementID: placement, ModelVersion: model, OccurredAt: now.Add(-ago),
		}
	}
	repo := perfRepo(t, []*warehouse.TrackingRecord{
		tracking("s1", "impression", "A", "S", "search_results", "hybrid@v1", 5*h),
		tracking("s2", "impression", "B", "S", "search_results", "hybrid@v1", 5*h),
		tracking("sc", "click", "A", "S", "", "", 4*h),
		tracking("r1", "impression", "A", "R", "similar_items", "als-v3", 5*h),
		tracking("r2", "impression", "A", "H", "home_feed", "M1", 5*h),
	})
	svc := mustHandler(repo, rpc.WithClock(func() time.Time { return now }))
	resp, err := svc.GetSearchPerformance(asPrincipal("admin-1", "user", "admin"), &analyticsv1.GetSearchPerformanceRequest{WindowHours: 24})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("rows = %+v, want exactly the search_results row", resp.Rows)
	}
	r := resp.Rows[0]
	if r.PlacementId != "search_results" || r.ModelVersion != "hybrid@v1" || r.ItemImpressions != 2 || r.Clicks != 1 || r.Ctr != 0.5 {
		t.Fatalf("row = %+v, want search_results/hybrid@v1 with 2 item impressions, 1 click, ctr 0.5", r)
	}
}

func TestGetSearchPerformance_WindowBoundaryInclusiveThroughHandler(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	repo := perfRepo(t, []*warehouse.TrackingRecord{
		{EventID: "edge", EventType: "impression", ListingID: "A", AnonymousID: "a", PrincipalType: "anonymous",
			ImpressionID: "I", PlacementID: "search_results", ModelVersion: "hybrid@v1", OccurredAt: now.Add(-168 * time.Hour)},
	})
	svc := mustHandler(repo, rpc.WithClock(func() time.Time { return now }))
	resp, err := svc.GetSearchPerformance(asPrincipal("admin-1", "user", "admin"), &analyticsv1.GetSearchPerformanceRequest{WindowHours: 168})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Rows) != 1 || resp.Rows[0].ItemImpressions != 1 {
		t.Fatalf("rows = %+v, want the impression exactly 168h old counted", resp.Rows)
	}
}

func TestGetSearchPerformance_RepositoryErrors(t *testing.T) {
	admin := asPrincipal("admin-1", "user", "admin")
	req := &analyticsv1.GetSearchPerformanceRequest{}
	if _, err := mustHandler(&searchStub{searchErr: errors.New("boom")}).GetSearchPerformance(admin, req); status.Code(err) != codes.Internal {
		t.Errorf("repo error: code = %v, want Internal", status.Code(err))
	}
	if _, err := mustHandler(newRepo()).GetSearchPerformance(admin, req); status.Code(err) != codes.Unavailable {
		t.Errorf("no interface: code = %v, want Unavailable", status.Code(err))
	}
}
