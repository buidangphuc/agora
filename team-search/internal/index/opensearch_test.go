package index_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

// capturedRequest records what the OpenSearchIndex sent, so tests can assert the
// version-guard wire shape without a live cluster.
type capturedRequest struct {
	method string
	path   string
	query  string
	body   string
}

// fakeOpenSearch is an httptest server that captures the last request and replies
// with a caller-supplied status/body. A "/" info probe is answered generically.
func fakeOpenSearch(t *testing.T, status int, respBody string, cap *capturedRequest) *index.OpenSearchIndex {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"version":{"number":"2.11.0","distribution":"opensearch"}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.query = r.URL.RawQuery
		cap.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)

	idx, err := index.New(srv.URL, "listings")
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	return idx
}

// AD2: Upsert applies OpenSearch external versioning so an out-of-order write
// cannot overwrite newer state.
func TestUpsert_UsesExternalVersioning(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"result":"created"}`, &cap)

	err := idx.Upsert(context.Background(), index.ListingDoc{ID: "l1", Title: "Phone", Version: 42})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !strings.Contains(cap.query, "version_type=external") {
		t.Errorf("expected version_type=external in query, got %q", cap.query)
	}
	if !strings.Contains(cap.query, "version=42") {
		t.Errorf("expected version=42 in query, got %q", cap.query)
	}
}

// AD2: a stale/duplicate version is rejected by OpenSearch with 409; the index
// treats it as a no-op so the newer state is preserved.
func TestUpsert_StaleVersionConflictIsNoOp(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, http.StatusConflict, `{"error":"version_conflict_engine_exception"}`, &cap)

	if err := idx.Upsert(context.Background(), index.ListingDoc{ID: "l1", Version: 1}); err != nil {
		t.Fatalf("expected 409 conflict to be a no-op, got error: %v", err)
	}
}

// AD2 / SA-H5: a partial update must NOT doc_as_upsert (which resurrected
// tombstoned listings); it uses a scripted version guard instead.
func TestPartialUpdate_NoUpsertUsesVersionGuardScript(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"result":"updated"}`, &cap)

	err := idx.PartialUpdate(context.Background(), "l1", map[string]interface{}{
		"status":  "published",
		"version": int64(99),
	})
	if err != nil {
		t.Fatalf("PartialUpdate: %v", err)
	}
	if strings.Contains(cap.body, "doc_as_upsert") {
		t.Errorf("partial update must not use doc_as_upsert (resurrects tombstones): %s", cap.body)
	}
	if !strings.Contains(cap.body, "script") {
		t.Errorf("expected a scripted update with version guard, got: %s", cap.body)
	}
	if !strings.Contains(cap.body, `"version":99`) {
		t.Errorf("expected version param 99 in script params, got: %s", cap.body)
	}
}

// F2: Search must request the four facet aggregations and parse the returned
// aggregation block into Facets, with the keyed price/rating buckets emitted in
// a stable fixed order regardless of JSON map iteration.
func TestSearch_ParsesFacetAggregations(t *testing.T) {
	var cap capturedRequest
	respBody := `{
      "hits": {"total": {"value": 3}, "hits": [{"_id": "a1", "_score": 1.0, "_source": {"id": "a1"}}]},
      "aggregations": {
        "categories": {"buckets": [{"key": "cat_phones", "doc_count": 2}, {"key": "cat_tablets", "doc_count": 1}]},
        "sellers": {"buckets": [{"key": "seller_1", "doc_count": 3}]},
        "price_ranges": {"buckets": {"100000-500000": {"doc_count": 0}, "1000000+": {"doc_count": 1}, "0-100000": {"doc_count": 1}, "500000-1000000": {"doc_count": 1}}},
        "ratings": {"buckets": {"1": {"doc_count": 3}, "4": {"doc_count": 2}, "3": {"doc_count": 3}, "2": {"doc_count": 3}}}
      }
    }`
	idx := fakeOpenSearch(t, 200, respBody, &cap)

	res, err := idx.Search(context.Background(), "phone", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// The query body must request all four facet aggregations.
	for _, want := range []string{`"aggs"`, "category_id", "seller_id", "price_ranges", "ratings"} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("expected agg request body to contain %q, got: %s", want, cap.body)
		}
	}

	if len(res.Facets.Categories) != 2 || res.Facets.Categories[0].Key != "cat_phones" || res.Facets.Categories[0].Count != 2 {
		t.Errorf("categories facet mismatch: %+v", res.Facets.Categories)
	}
	if len(res.Facets.Sellers) != 1 || res.Facets.Sellers[0].Key != "seller_1" || res.Facets.Sellers[0].Count != 3 {
		t.Errorf("sellers facet mismatch: %+v", res.Facets.Sellers)
	}

	// Price ranges emitted in fixed order with the seeded counts.
	wantPrice := []index.FacetBucket{
		{Key: "0-100000", Count: 1},
		{Key: "100000-500000", Count: 0},
		{Key: "500000-1000000", Count: 1},
		{Key: "1000000+", Count: 1},
	}
	if len(res.Facets.PriceRanges) != len(wantPrice) {
		t.Fatalf("price_ranges len = %d, want %d: %+v", len(res.Facets.PriceRanges), len(wantPrice), res.Facets.PriceRanges)
	}
	for i, w := range wantPrice {
		if res.Facets.PriceRanges[i] != w {
			t.Errorf("price_ranges[%d] = %+v, want %+v", i, res.Facets.PriceRanges[i], w)
		}
	}

	// Ratings emitted in fixed floor order 4,3,2,1 (cumulative counts).
	wantRatings := []index.FacetBucket{
		{Key: "4", Count: 2},
		{Key: "3", Count: 3},
		{Key: "2", Count: 3},
		{Key: "1", Count: 3},
	}
	for i, w := range wantRatings {
		if res.Facets.Ratings[i] != w {
			t.Errorf("ratings[%d] = %+v, want %+v", i, res.Facets.Ratings[i], w)
		}
	}
}

// F2: an empty result set must yield empty — never nil — facet buckets, and the
// fixed price/rating buckets are still present with zero counts (no nil-panic).
func TestSearch_EmptyResultSafeFacets(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)

	res, err := idx.Search(context.Background(), "nomatch", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Facets.Categories == nil || res.Facets.Sellers == nil ||
		res.Facets.PriceRanges == nil || res.Facets.Ratings == nil {
		t.Fatalf("facet buckets must be non-nil on empty result: %+v", res.Facets)
	}
	if len(res.Facets.Categories) != 0 || len(res.Facets.Sellers) != 0 {
		t.Errorf("terms facets should be empty on empty result: %+v", res.Facets)
	}
	if len(res.Facets.PriceRanges) != 4 || len(res.Facets.Ratings) != 4 {
		t.Errorf("fixed facets should keep their buckets (zero counts): %+v", res.Facets)
	}
	for _, b := range append(append([]index.FacetBucket{}, res.Facets.PriceRanges...), res.Facets.Ratings...) {
		if b.Count != 0 {
			t.Errorf("expected zero count on empty result, got %+v", b)
		}
	}
}

// F2: min_rating adds a rating>=N range filter, constraining the matched set the
// facets count over.
func TestSearch_MinRatingAddsFilter(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)

	if _, err := idx.Search(context.Background(), "", nil, "", 0, 0, 4, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(cap.body, "rating") {
		t.Errorf("expected a rating filter in the query body, got: %s", cap.body)
	}
}

// AD2 / SA-H5: a partial update for a deleted (tombstoned) listing returns 404
// from OpenSearch and must be a no-op — the doc is NOT recreated.
func TestPartialUpdate_TombstonedListingNotResurrected(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, http.StatusNotFound, `{"error":"document_missing_exception"}`, &cap)

	err := idx.PartialUpdate(context.Background(), "deleted-1", map[string]interface{}{
		"status":  "published",
		"version": int64(5),
	})
	if err != nil {
		t.Fatalf("expected 404 (missing doc) to be a no-op, got error: %v", err)
	}
}
