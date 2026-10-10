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

// D1: Upsert is one scripted_upsert guarded on _source.version, not an
// external-version index request.
func TestUpsert_IsGuardedScriptedUpsert(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"result":"created"}`, &cap)

	if err := idx.Upsert(context.Background(), index.ListingDoc{ID: "l1", Title: "Phone", Version: 42}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cap.path != "/listings/_update/l1" {
		t.Errorf("expected the update API, got %s %s", cap.method, cap.path)
	}
	if strings.Contains(cap.query, "version_type") {
		t.Errorf("external versioning must be gone, got query %q", cap.query)
	}
	for _, want := range []string{`"scripted_upsert":true`, `"kind":"upsert"`, `"version":42`, `"title":"Phone"`} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("expected %s in body, got %s", want, cap.body)
		}
	}
	if strings.Contains(cap.body, `"rating"`) {
		t.Errorf("rating must no longer be written (D9): %s", cap.body)
	}
}

// D5: Delete writes the tombstone through the same script with the delete's version.
func TestDelete_WritesVersionedTombstone(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"result":"updated"}`, &cap)

	if err := idx.Delete(context.Background(), "l1", 77); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if cap.method != http.MethodPost || cap.path != "/listings/_update/l1" {
		t.Errorf("expected a scripted update, got %s %s", cap.method, cap.path)
	}
	for _, want := range []string{`"scripted_upsert":true`, `"kind":"delete"`, `"version":77`, `"now":`} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("expected %s in body, got %s", want, cap.body)
		}
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

// F2: Search must request the facet aggregations and parse the returned
// aggregation block into Facets, with the keyed price buckets emitted in a
// stable fixed order regardless of JSON map iteration. D9: no ratings
// aggregation is requested and the ratings facet is an empty list.
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

	for _, want := range []string{`"aggs"`, "category_id", "seller_id", "price_ranges"} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("expected agg request body to contain %q, got: %s", want, cap.body)
		}
	}
	if strings.Contains(cap.body, "ratings") {
		t.Errorf("the ratings aggregation must be gone (D9): %s", cap.body)
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

	// Even if a cluster returned a ratings block, the facet stays empty.
	if res.Facets.Ratings == nil || len(res.Facets.Ratings) != 0 {
		t.Errorf("ratings facet must be an empty non-nil list: %+v", res.Facets.Ratings)
	}
}

// F2: an empty result set must yield empty — never nil — facet buckets, and the
// fixed price buckets are still present with zero counts (no nil-panic).
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
	if len(res.Facets.PriceRanges) != 4 || len(res.Facets.Ratings) != 0 {
		t.Errorf("price buckets kept with zero counts, ratings empty: %+v", res.Facets)
	}
	for _, b := range res.Facets.PriceRanges {
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

// Visibility default: with no status filter a status=published clause is added,
// so drafts/rejected/unspecified listings never match (hits, total and facets).
func TestSearch_DefaultsToPublishedOnly(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
	if _, err := idx.Search(context.Background(), "phone", map[string]string{"seller_id": "s1"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(cap.body, `{"term":{"status":"published"}}`) {
		t.Errorf("expected published clause, got %s", cap.body)
	}
}

func TestSearchVector_DefaultsToPublishedOnly(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
	if _, err := idx.SearchVector(context.Background(), []float32{0.1}, nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	if !strings.Contains(cap.body, `{"term":{"status":"published"}}`) {
		t.Errorf("expected published clause in knn filter, got %s", cap.body)
	}
}

// An explicit status is used as is (the handler authorises it); no second clause.
func TestSearch_ExplicitStatusReplacesDefault(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
	if _, err := idx.Search(context.Background(), "", map[string]string{"status": "draft", "seller_id": "A"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if strings.Contains(cap.body, `"status":"published"`) || !strings.Contains(cap.body, `"status":"draft"`) {
		t.Errorf("expected only the draft status clause, got %s", cap.body)
	}
}

// Suggestions are public: titles of non-published listings must not leak.
func TestSuggest_PublishedOnly(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
	if _, err := idx.Suggest(context.Background(), "iph", 5); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if !strings.Contains(cap.body, `{"term":{"status":"published"}}`) {
		t.Errorf("expected published clause in suggest, got %s", cap.body)
	}
}

// D7: SORT_BY_NEWEST sorts both legs by created_at desc (missing last), then the
// id keyword asc; never by _id.
func TestSortNewest_BothLegsUseCreatedAtThenID(t *testing.T) {
	const want = `"sort":[{"created_at":{"missing":"_last","order":"desc","unmapped_type":"date"}},{"id":{"order":"asc"}}]`
	for _, leg := range []string{"lexical", "vector"} {
		var cap capturedRequest
		idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
		var err error
		if leg == "lexical" {
			_, err = idx.Search(context.Background(), "x", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_NEWEST, 0, 10)
		} else {
			_, err = idx.SearchVector(context.Background(), []float32{0.1}, nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_NEWEST, 0, 10)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cap.body, want) || strings.Contains(cap.body, `"_id"`) {
			t.Errorf("%s sort body: %s", leg, cap.body)
		}
	}
}

// D4: in_stock=true becomes range stock > 0 in both legs (the k-NN leg inside
// knn.filter), never a raw term on a non-existent field.
func TestInStock_BecomesStockRangeInBothLegs(t *testing.T) {
	for _, leg := range []string{"lexical", "vector"} {
		var cap capturedRequest
		idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
		f := map[string]string{"in_stock": "true"}
		var err error
		if leg == "lexical" {
			_, err = idx.Search(context.Background(), "x", f, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		} else {
			_, err = idx.SearchVector(context.Background(), []float32{0.1}, f, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cap.body, `{"range":{"stock":{"gt":0}}}`) {
			t.Errorf("%s: no stock range clause: %s", leg, cap.body)
		}
		if strings.Contains(cap.body, `"in_stock"`) {
			t.Errorf("%s: in_stock leaked as a raw term: %s", leg, cap.body)
		}
		if leg == "vector" && !strings.Contains(cap.body, `"knn":{"embedding":{"filter":{"bool":{"filter":[`) {
			t.Errorf("vector: filter not inside knn.filter: %s", cap.body)
		}
	}
}

// D3: stock is decoded from _source with presence (absent stays nil).
func TestSearch_DecodesStockFromSource(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits":{"total":{"value":3},"hits":[
	  {"_id":"a","_source":{"id":"a","stock":4}},
	  {"_id":"b","_source":{"id":"b","stock":0}},
	  {"_id":"c","_source":{"id":"c"}}]}}`, &cap)
	for _, leg := range []string{"lexical", "vector"} {
		var res index.SearchResult
		var err error
		if leg == "lexical" {
			res, err = idx.Search(context.Background(), "x", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		} else {
			res, err = idx.SearchVector(context.Background(), []float32{0.1}, nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		}
		if err != nil {
			t.Fatal(err)
		}
		h := res.Hits
		if len(h) != 3 || h[0].Stock == nil || *h[0].Stock != 4 || h[1].Stock == nil || *h[1].Stock != 0 || h[2].Stock != nil {
			t.Errorf("%s: stock decode wrong: %+v", leg, h)
		}
	}
}

// Every structured filter the lexical leg applies also constrains the k-NN leg,
// inside knn.filter so the k neighbours are found among matching listings.
func TestFilters_SameClausesInBothLegs(t *testing.T) {
	f := map[string]string{
		"status": "published", "in_stock": "true", "seller_id": "s1",
		"tag.connectivity": "bluetooth-5-3", "sku.color": "den",
	}
	run := func(vector bool) string {
		var cap capturedRequest
		idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
		var err error
		if vector {
			_, err = idx.SearchVector(context.Background(), []float32{0.1}, f, "c1", 100, 900, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		} else {
			_, err = idx.Search(context.Background(), "x", f, "c1", 100, 900, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
		}
		if err != nil {
			t.Fatal(err)
		}
		return cap.body
	}
	vec := run(true)
	for _, want := range []string{
		`{"term":{"status":"published"}}`, `{"range":{"stock":{"gt":0}}}`, `{"term":{"seller_id":"s1"}}`,
		`{"term":{"category_id":"c1"}}`, `{"range":{"price":{"gte":100,"lte":900}}}`,
		`bluetooth-5-3`, `"nested"`, `den`,
	} {
		if !strings.Contains(vec, want) {
			t.Errorf("k-NN leg is missing filter %s: %s", want, vec)
		}
		if lex := run(false); !strings.Contains(lex, want) {
			t.Errorf("lexical leg is missing filter %s", want)
		}
	}
	if !strings.Contains(vec, `"knn":{"embedding":{"filter":{"bool":{"filter":[`) {
		t.Errorf("filters are not inside knn.filter (post-filtering): %s", vec)
	}
}

// FacetsForIDs aggregates over exactly the given ids under the same filters.
func TestFacetsForIDs_RestrictsToIDsAndFilters(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits": {"total": {"value": 0}, "hits": []}}`, &cap)
	if _, err := idx.FacetsForIDs(context.Background(), []string{"a", "b"}, map[string]string{"seller_id": "s1"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"terms":{"id":["a","b"]}`, `{"term":{"seller_id":"s1"}}`, `{"term":{"status":"published"}}`, `"size":0`, `"price_ranges"`} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("missing %s in %s", want, cap.body)
		}
	}
}
