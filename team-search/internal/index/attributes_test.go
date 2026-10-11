package index_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func searchBody(t *testing.T, filters map[string]string) map[string]any {
	t.Helper()
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits":{"total":{"value":0},"hits":[]},"aggregations":{}}`, &cap)
	if _, err := idx.Search(context.Background(), "", filters, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(cap.body), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// tag.<group> is a term on the flat facet_tags keyword, never a raw field term.
func TestSearch_TagFilterBecomesFacetTagsTerm(t *testing.T) {
	raw, _ := json.Marshal(searchBody(t, map[string]string{"tag.connectivity": "bluetooth-5-3"}))
	if !strings.Contains(string(raw), `{"term":{"facet_tags":"connectivity:bluetooth-5-3"}}`) {
		t.Errorf("tag filter clause missing: %s", raw)
	}
	if strings.Contains(string(raw), `"tag.connectivity"`) {
		t.Errorf("tag.* must not reach OpenSearch as a field name: %s", raw)
	}
}

// Every sku.<group> lands in ONE nested clause with is_in_stock, so the groups
// must hold on the same variant (no cross-variant false positives).
func TestSearch_SkuFiltersShareOneNestedClause(t *testing.T) {
	raw, _ := json.Marshal(searchBody(t, map[string]string{"sku.color": "xanh-navy", "sku.capacity": "512gb,1tb"}))
	s := string(raw)
	if got := strings.Count(s, `"nested":{"path":"skus"`); got != 2 { // 1 query clause + 1 aggregation
		t.Errorf("want one nested query clause (+1 nested agg), got %d nested: %s", got, s)
	}
	for _, want := range []string{
		`{"term":{"skus.is_in_stock":true}}`,
		`{"term":{"skus.attrs":"color:xanh-navy"}}`,
		`{"terms":{"skus.attrs":["capacity:512gb","capacity:1tb"]}}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
}

// A malformed group or value cannot become a field name: it matches nothing.
func TestSearch_MalformedAttrFilterMatchesNothing(t *testing.T) {
	for k, v := range map[string]string{"tag.Bad Group": "x", "sku.color": "Xanh Navy", "tag.color": ""} {
		raw, _ := json.Marshal(searchBody(t, map[string]string{k: v}))
		if !strings.Contains(string(raw), `"match_none"`) {
			t.Errorf("%s=%q: want match_none, got %s", k, v, raw)
		}
	}
}

func TestSearch_DynamicFacetsParsedPerGroup(t *testing.T) {
	var cap capturedRequest
	resp := `{
	  "hits": {"total": {"value": 3}, "hits": []},
	  "aggregations": {
	    "tag_facets": {"buckets": [
	      {"key": "connectivity:bluetooth-5-3", "doc_count": 3},
	      {"key": "feature:chong-nuoc", "doc_count": 2},
	      {"key": "connectivity:wifi-6", "doc_count": 1}]},
	    "sku_facets": {"doc_count": 6, "available": {"doc_count": 4, "attrs": {"buckets": [
	      {"key": "color:xanh-navy", "doc_count": 2, "listings": {"doc_count": 2}},
	      {"key": "capacity:512gb", "doc_count": 3, "listings": {"doc_count": 2}},
	      {"key": "color:den", "doc_count": 1, "listings": {"doc_count": 1}}]}}}
	  }
	}`
	idx := fakeOpenSearch(t, 200, resp, &cap)
	res, err := idx.Search(context.Background(), "", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	wantTags := []index.AttributeFacet{
		{Group: "connectivity", Buckets: []index.FacetBucket{{Key: "bluetooth-5-3", Count: 3}, {Key: "wifi-6", Count: 1}}},
		{Group: "feature", Buckets: []index.FacetBucket{{Key: "chong-nuoc", Count: 2}}},
	}
	wantSkus := []index.AttributeFacet{
		{Group: "color", Buckets: []index.FacetBucket{{Key: "xanh-navy", Count: 2}, {Key: "den", Count: 1}}},
		{Group: "capacity", Buckets: []index.FacetBucket{{Key: "512gb", Count: 2}}}, // listings, not variants
	}
	if got, _ := json.Marshal(res.Facets.Tags); string(got) != mustJSON(wantTags) {
		t.Errorf("tags = %s, want %s", got, mustJSON(wantTags))
	}
	if got, _ := json.Marshal(res.Facets.SKUs); string(got) != mustJSON(wantSkus) {
		t.Errorf("skus = %s, want %s", got, mustJSON(wantSkus))
	}
}

func TestSearch_EmptyResultHasNonNilDynamicFacets(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"hits":{"total":{"value":0},"hits":[]},"aggregations":{}}`, &cap)
	res, err := idx.Search(context.Background(), "", nil, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Facets.Tags == nil || res.Facets.SKUs == nil || len(res.Facets.Tags)+len(res.Facets.SKUs) != 0 {
		t.Errorf("want empty non-nil dynamic facets: %+v", res.Facets)
	}
}

func TestUpsert_ScriptKeepsStoredTagsWhenClassifierFailed(t *testing.T) {
	var cap capturedRequest
	idx := fakeOpenSearch(t, 200, `{"result":"updated"}`, &cap)
	if err := idx.Upsert(context.Background(), index.ListingDoc{ID: "l1", Version: 5, TagsPending: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"oldFacetTags", "oldSkus", "tags_pending", `"tags_pending":true`} {
		if !strings.Contains(cap.body, want) {
			t.Errorf("write request lacks %q: %s", want, cap.body)
		}
	}
}

func TestParseAttrFilterKey(t *testing.T) {
	cases := []struct {
		key, prefix, group string
		ok, bad            bool
	}{
		{"tag.color", "tag.", "color", true, false},
		{"sku.capacity", "sku.", "capacity", true, false},
		{"category_id", "", "", false, false},
		{"tag.", "tag.", "", true, true},
		{"sku.Bad", "sku.", "", true, true},
		{"tag.a b", "tag.", "", true, true},
	}
	for _, c := range cases {
		p, g, ok, err := index.ParseAttrFilterKey(c.key)
		if ok != c.ok || p != c.prefix || g != c.group || (err != nil) != c.bad {
			t.Errorf("%q => (%q,%q,%v,%v)", c.key, p, g, ok, err)
		}
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
