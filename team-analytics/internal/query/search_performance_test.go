package query_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

const searchPlacement = "search_results"

var searchNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func searchRepo(t *testing.T, recs []*warehouse.TrackingRecord, orders ...*warehouse.OrderFactRecord) *query.DuckDBRepository {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	if err := w.Write(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOrderFacts(context.Background(), orders); err != nil {
		t.Fatal(err)
	}
	return query.NewDuckDBRepository(w.DB())
}

func sev(id, typ, user, listing, imp, placement, model string, at time.Time) *warehouse.TrackingRecord {
	return &warehouse.TrackingRecord{
		EventID: id, EventType: typ, ListingID: listing, AnonymousID: "anon-" + user,
		PrincipalID: user, PrincipalType: "user", ImpressionID: imp, PlacementID: placement,
		ModelVersion: model, OccurredAt: at,
	}
}

func spaid(id, buyer, listing string, at time.Time) *warehouse.OrderFactRecord {
	return &warehouse.OrderFactRecord{
		EventID: id, OrderID: "o-" + id, ListingID: listing, SellerID: "s", Quantity: 1,
		UnitPrice: 1, Currency: "VND", OccurredAt: at, Status: "PAID", BuyerID: buyer,
	}
}

func shown(prefix, user, imp, model string, n int, at time.Time) []*warehouse.TrackingRecord {
	var out []*warehouse.TrackingRecord
	for i := 1; i <= n; i++ {
		out = append(out, sev(fmt.Sprintf("%s-i%d", prefix, i), "impression", user, fmt.Sprintf("%s%d", prefix, i), imp, searchPlacement, model, at))
	}
	return out
}

func searchByModel(t *testing.T, repo *query.DuckDBRepository, since, until time.Time, attr int) map[string]query.PerformanceRow {
	t.Helper()
	got, err := repo.SearchPerformance(context.Background(), since, until, attr)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]query.PerformanceRow{}
	for _, r := range got {
		if r.PlacementID != searchPlacement {
			t.Fatalf("row %+v has placement other than %s", r, searchPlacement)
		}
		if _, dup := by[r.ModelVersion]; dup {
			t.Fatalf("duplicate row for model %s: %+v", r.ModelVersion, got)
		}
		by[r.ModelVersion] = r
	}
	return by
}

func TestSearchPerformance_CTRPerModelVersion(t *testing.T) {
	h := time.Hour
	recs := shown("a", "u1", "A", "hybrid@v1", 10, searchNow.Add(-5*h))
	recs = append(recs, shown("b", "u2", "B", "lexical@v1", 10, searchNow.Add(-5*h))...)
	recs = append(recs,
		sev("ca1", "click", "u1", "a1", "A", "", "", searchNow.Add(-4*h)),
		sev("ca2", "click", "u1", "a2", "A", "", "", searchNow.Add(-4*h)),
		sev("cb1", "click", "u2", "b1", "B", "", "", searchNow.Add(-4*h)),
	)
	by := searchByModel(t, searchRepo(t, recs), searchNow.Add(-24*h), searchNow, 24)
	if len(by) != 2 {
		t.Fatalf("rows = %+v, want one per model version", by)
	}
	hy, lx := by["hybrid@v1"], by["lexical@v1"]
	if hy.Impressions != 1 || hy.ItemImpressions != 10 || hy.Clicks != 2 {
		t.Fatalf("hybrid@v1 = %+v, want 1 impression, 10 item impressions, 2 clicks", hy)
	}
	if lx.Impressions != 1 || lx.ItemImpressions != 10 || lx.Clicks != 1 {
		t.Fatalf("lexical@v1 = %+v, want 1 impression, 10 item impressions, 1 click", lx)
	}
}

func TestSearchPerformance_OnlySearchResultsAreCounted(t *testing.T) {
	h := time.Hour
	recs := []*warehouse.TrackingRecord{
		sev("s1", "impression", "u1", "l1", "S", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("r1", "impression", "u1", "l1", "R", "similar_items", "als-v3", searchNow.Add(-5*h)),
		sev("r2", "impression", "u1", "l2", "R", "similar_items", "als-v3", searchNow.Add(-5*h)),
		sev("rc", "click", "u1", "l1", "R", "", "", searchNow.Add(-4*h)),
	}
	by := searchByModel(t, searchRepo(t, recs), searchNow.Add(-24*h), searchNow, 24)
	if _, ok := by["als-v3"]; ok {
		t.Fatalf("similar_items model appeared in search rows: %+v", by)
	}
	if len(by) != 1 || by["hybrid@v1"].ItemImpressions != 1 || by["hybrid@v1"].Clicks != 0 {
		t.Fatalf("rows = %+v, want only the untouched hybrid@v1 row", by)
	}
}

func TestSearchPerformance_ClickOnListingNotShownIsNotCredited(t *testing.T) {
	h := time.Hour
	recs := []*warehouse.TrackingRecord{
		sev("i", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("c-ok", "click", "u1", "A", "I", "", "", searchNow.Add(-4*h)),
		sev("c-other", "click", "u1", "B", "I", "", "", searchNow.Add(-4*h)),
	}
	r := searchByModel(t, searchRepo(t, recs), searchNow.Add(-24*h), searchNow, 24)["hybrid@v1"]
	if r.Impressions != 1 || r.Clicks != 1 {
		t.Fatalf("row = %+v, want exactly the click on the shown listing", r)
	}
	only := []*warehouse.TrackingRecord{
		sev("i", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("c-other", "click", "u1", "B", "I", "", "", searchNow.Add(-4*h)),
	}
	r = searchByModel(t, searchRepo(t, only), searchNow.Add(-24*h), searchNow, 24)["hybrid@v1"]
	if r.Clicks != 0 {
		t.Fatalf("row = %+v, want no click credited", r)
	}
}

func TestSearchPerformance_ClickBeforeImpressionIsNotCredited(t *testing.T) {
	h := time.Hour
	recs := []*warehouse.TrackingRecord{
		sev("i", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("c-early", "click", "u1", "A", "I", "", "", searchNow.Add(-6*h)),
	}
	r := searchByModel(t, searchRepo(t, recs), searchNow.Add(-24*h), searchNow, 24)["hybrid@v1"]
	if r.Impressions != 1 || r.Clicks != 0 {
		t.Fatalf("row = %+v, want the impression with no click", r)
	}
}

func TestSearchPerformance_ReusedImpressionIDUnderAnotherModelIsSeparate(t *testing.T) {
	h := time.Hour
	recs := []*warehouse.TrackingRecord{
		sev("i1", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("i2", "impression", "u1", "A", "I", searchPlacement, "lexical@v1", searchNow.Add(-4*h)),
		sev("c1", "click", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-3*h)),
		sev("c2", "click", "u1", "A", "I", "", "", searchNow.Add(-2*h)),
	}
	by := searchByModel(t, searchRepo(t, recs), searchNow.Add(-24*h), searchNow, 24)
	if len(by) != 2 {
		t.Fatalf("rows = %+v, want one per model version", by)
	}
	hy, lx := by["hybrid@v1"], by["lexical@v1"]
	if hy.Impressions != 1 || hy.Clicks != 1 || lx.Impressions != 1 || lx.Clicks != 1 {
		t.Fatalf("hybrid = %+v, lexical = %+v; want 1 impression and 1 click each", hy, lx)
	}
}

func TestSearchPerformance_PurchasesComeFromServerOrderFacts(t *testing.T) {
	h := time.Hour
	base := []*warehouse.TrackingRecord{
		sev("i", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
		sev("c", "click", "u1", "A", "I", "", "", searchNow.Add(-4*h)),
		sev("beacon", "purchase", "u1", "A", "", "", "", searchNow.Add(-3*h)),
	}
	if r := searchByModel(t, searchRepo(t, base), searchNow.Add(-24*h), searchNow, 24)["hybrid@v1"]; r.Clicks != 1 || r.Purchases != 0 {
		t.Fatalf("row = %+v, a purchase beacon without an order must leave purchases at 0", r)
	}
	repo := searchRepo(t, base,
		spaid("ok", "u1", "A", searchNow.Add(-3*h)),
		spaid("other-buyer", "u2", "A", searchNow.Add(-3*h)),
		spaid("other-listing", "u1", "B", searchNow.Add(-3*h)),
		spaid("before-click", "u1", "A", searchNow.Add(-5*h)),
		spaid("outside-attribution", "u1", "A", searchNow.Add(-1*h)),
	)
	if r := searchByModel(t, repo, searchNow.Add(-24*h), searchNow, 1)["hybrid@v1"]; r.Purchases != 1 {
		t.Fatalf("purchases = %d, want only the in-window PAID line of the clicker on the clicked listing", r.Purchases)
	}
}

func TestSearchPerformance_RowFieldsAndAttributionWindow(t *testing.T) {
	h := time.Hour
	recs := shown("a", "u1", "A", "hybrid@v1", 10, searchNow.Add(-40*h))
	recs = append(recs,
		sev("c1", "click", "u1", "a1", "A", "", "", searchNow.Add(-30*h)),
		sev("c2", "click", "u1", "a2", "A", "", "", searchNow.Add(-30*h)),
		sev("c3", "click", "u1", "a3", "A", "", "", searchNow.Add(-30*h)),
		sev("c4", "click", "u1", "a4", "A", "", "", searchNow.Add(-30*h)),
	)
	repo := searchRepo(t, recs,
		spaid("p1", "u1", "a1", searchNow.Add(-29*h)),
		spaid("p2", "u1", "a2", searchNow.Add(-29*h)),
	)
	r := searchByModel(t, repo, searchNow.Add(-48*h), searchNow, 24)["hybrid@v1"]
	want := query.PerformanceRow{
		PlacementID: searchPlacement, ModelVersion: "hybrid@v1", Impressions: 1, ItemImpressions: 10,
		Clicks: 4, AddToCarts: 0, Purchases: 2, MatureClicks: 4, MaturePurchases: 2,
	}
	if r != want {
		t.Fatalf("row = %+v, want %+v", r, want)
	}
}

func TestSearchPerformance_EmptyWindowReturnsNoRows(t *testing.T) {
	h := time.Hour
	recs := []*warehouse.TrackingRecord{
		sev("i", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", searchNow.Add(-5*h)),
	}
	got, err := searchRepo(t, recs).SearchPerformance(context.Background(), searchNow.Add(24*h), searchNow.Add(25*h), 24)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v; want zero rows and no error", got, err)
	}
	none, err := searchRepo(t, nil).SearchPerformance(context.Background(), searchNow.Add(-24*h), searchNow, 24)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty warehouse: got %+v, %v; want zero rows and no error", none, err)
	}
}

func TestSearchPerformance_WindowBoundaryIsInclusive(t *testing.T) {
	h := time.Hour
	since := searchNow.Add(-168 * h)
	recs := []*warehouse.TrackingRecord{
		sev("i-edge", "impression", "u1", "A", "I", searchPlacement, "hybrid@v1", since),
		sev("i-out", "impression", "u1", "B", "J", searchPlacement, "lexical@v1", since.Add(-time.Second)),
	}
	by := searchByModel(t, searchRepo(t, recs), since, searchNow, 24)
	if r, ok := by["hybrid@v1"]; !ok || r.Impressions != 1 {
		t.Fatalf("rows = %+v, want the impression exactly 168h old counted", by)
	}
	if _, ok := by["lexical@v1"]; ok {
		t.Fatalf("rows = %+v, an impression just outside the window must not count", by)
	}
}
