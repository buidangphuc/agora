package index_test

import (
	"context"
	"net/http"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

func phone(id string, version int64, tags []string, skus ...index.SkuDoc) index.ListingDoc {
	d := liveDoc(id, "dien thoai "+id, version)
	d.Status = "published"
	d.FacetTags = tags
	d.SKUs = skus
	return d
}

func sku(id string, stock int32, attrs ...string) index.SkuDoc {
	return index.SkuDoc{VariantID: id, Stock: stock, InStock: stock > 0, Attrs: attrs}
}

func ids(res index.SearchResult) map[string]bool {
	out := map[string]bool{}
	for _, h := range res.Hits {
		out[h.ListingID] = true
	}
	return out
}

func find(fs []index.AttributeFacet, group string) map[string]int64 {
	out := map[string]int64{}
	for _, f := range fs {
		if f.Group == group {
			for _, b := range f.Buckets {
				out[b.Key] = b.Count
			}
		}
	}
	return out
}

func srch(t *testing.T, idx *index.OpenSearchIndex, f map[string]string) index.SearchResult {
	t.Helper()
	res, err := idx.Search(context.Background(), "", f, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 20)
	if err != nil {
		t.Fatalf("Search %v: %v", f, err)
	}
	return res
}

// Fresh index: skus is nested, facet_tags keyword.
func TestIT_Attr_FreshMappingHasNestedSkus(t *testing.T) {
	_, url, name := readyIndex(t)
	if got := mappedType(t, url, name, "skus"); got != "nested" {
		t.Errorf("skus type = %q, want nested", got)
	}
	if got := mappedType(t, url, name, "facet_tags"); got != "keyword" {
		t.Errorf("facet_tags type = %q, want keyword", got)
	}
}

// An index created by the previous release gains the new fields in place; a
// second EnsureIndex is a no-op and existing documents are untouched.
func TestIT_Attr_ExistingIndexGainsNestedSkus(t *testing.T) {
	idx, url, name := itIndex(t)
	if code, body := osDo(t, http.MethodPut, url+"/"+name,
		`{"mappings":{"properties":{"id":{"type":"keyword"},"version":{"type":"long"},"status":{"type":"keyword"}}}}`); code != 200 {
		t.Fatalf("create old index: %d %s", code, body)
	}
	if code, body := osDo(t, http.MethodPut, url+"/"+name+"/_doc/old?refresh=true", `{"id":"old","version":1,"status":"published"}`); code >= 300 {
		t.Fatalf("seed old doc: %d %s", code, body)
	}
	for i := 0; i < 2; i++ {
		if err := idx.EnsureIndex(context.Background()); err != nil {
			t.Fatalf("EnsureIndex #%d: %v", i+1, err)
		}
	}
	if got := mappedType(t, url, name, "skus"); got != "nested" {
		t.Errorf("skus type = %q, want nested", got)
	}
	// The old document has no tags: it simply never matches a facet filter.
	if res := srch(t, idx, map[string]string{"tag.color": "den"}); res.Total != 0 {
		t.Errorf("untagged old doc matched a tag filter: %+v", res.Hits)
	}
	if res := srch(t, idx, nil); res.Total != 1 {
		t.Errorf("old doc lost: total=%d", res.Total)
	}
}

// The headline guarantee: sku filters hold on ONE in-stock variant.
func TestIT_Attr_SkuFiltersDoNotMatchAcrossVariants(t *testing.T) {
	idx, _, _ := readyIndex(t)
	// a: navy/256 and black/512 (no navy+512 variant)  b: navy/512 in stock
	// c: navy/512 but sold out
	upsert(t, idx, phone("a", 10, []string{"connectivity:bluetooth-5-3"},
		sku("a1", 3, "color:xanh-navy", "capacity:256gb"), sku("a2", 3, "color:den", "capacity:512gb")))
	upsert(t, idx, phone("b", 10, []string{"connectivity:bluetooth-5-3"},
		sku("b1", 2, "color:xanh-navy", "capacity:512gb")))
	upsert(t, idx, phone("c", 10, []string{"connectivity:wifi-6"},
		sku("c1", 0, "color:xanh-navy", "capacity:512gb"), sku("c2", 4, "color:den", "capacity:256gb")))

	got := ids(srch(t, idx, map[string]string{"sku.color": "xanh-navy", "sku.capacity": "512gb"}))
	if len(got) != 1 || !got["b"] {
		t.Errorf("navy+512gb = %v, want only b (a is a cross-variant false positive, c's variant is sold out)", got)
	}
	got = ids(srch(t, idx, map[string]string{"sku.color": "xanh-navy"}))
	if len(got) != 2 || !got["a"] || !got["b"] {
		t.Errorf("navy = %v, want a and b (c's navy is sold out)", got)
	}
	got = ids(srch(t, idx, map[string]string{"sku.capacity": "256gb,512gb"}))
	if len(got) != 3 {
		t.Errorf("256gb|512gb = %v, want all three", got)
	}
}

func TestIT_Attr_TagFilterAndCombinedWithSku(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, phone("a", 10, []string{"connectivity:bluetooth-5-3", "feature:chong-nuoc"}, sku("a1", 1, "color:den")))
	upsert(t, idx, phone("b", 10, []string{"connectivity:wifi-6"}, sku("b1", 1, "color:den")))
	if got := ids(srch(t, idx, map[string]string{"tag.connectivity": "bluetooth-5-3"})); len(got) != 1 || !got["a"] {
		t.Errorf("bluetooth = %v, want a", got)
	}
	if got := ids(srch(t, idx, map[string]string{"tag.connectivity": "bluetooth-5-3,wifi-6"})); len(got) != 2 {
		t.Errorf("bluetooth|wifi = %v, want a and b", got)
	}
	if got := ids(srch(t, idx, map[string]string{"tag.feature": "chong-nuoc", "sku.color": "den"})); len(got) != 1 || !got["a"] {
		t.Errorf("chong-nuoc + den = %v, want a", got)
	}
	// Existing filters keep working beside the new ones.
	if got := ids(srch(t, idx, map[string]string{"tag.connectivity": "wifi-6", "seller_id": "nobody"})); len(got) != 0 {
		t.Errorf("seller_id must still AND with tag filters: %v", got)
	}
}

func TestIT_Attr_FacetsCountListingsAndOnlyInStockVariants(t *testing.T) {
	idx, _, _ := readyIndex(t)
	upsert(t, idx, phone("a", 10, []string{"connectivity:bluetooth-5-3"},
		sku("a1", 3, "color:den", "capacity:256gb"), sku("a2", 3, "color:den", "capacity:512gb")))
	upsert(t, idx, phone("b", 10, []string{"connectivity:bluetooth-5-3"},
		sku("b1", 2, "color:den", "capacity:512gb"), sku("b2", 0, "color:trang", "capacity:512gb")))
	res := srch(t, idx, nil)
	if got := find(res.Facets.Tags, "connectivity")["bluetooth-5-3"]; got != 2 {
		t.Errorf("bluetooth listings = %d, want 2", got)
	}
	colors := find(res.Facets.SKUs, "color")
	if colors["den"] != 2 { // a has two den variants but counts once
		t.Errorf("den listings = %d, want 2 (listings, not variants): %v", colors["den"], colors)
	}
	if _, sold := colors["trang"]; sold {
		t.Errorf("a sold-out variant must not produce a facet value: %v", colors)
	}
	if caps := find(res.Facets.SKUs, "capacity"); caps["256gb"] != 1 || caps["512gb"] != 2 {
		t.Errorf("capacity facets = %v, want 256gb:1 512gb:2", caps)
	}
	// With sku.capacity=256gb selected only matching variants are counted.
	narrowed := srch(t, idx, map[string]string{"sku.capacity": "256gb"})
	if colors := find(narrowed.Facets.SKUs, "color"); colors["den"] != 1 || len(colors) != 1 {
		t.Errorf("color facets under 256gb = %v, want only den:1", colors)
	}
}

// A hybrid-era document (vector, no tags) and a tagged one coexist; the k-NN leg
// accepts the new filters.
func TestIT_Attr_KnnLegAcceptsAttrFilters(t *testing.T) {
	idx, _, _ := readyIndex(t)
	d := phone("a", 10, []string{"connectivity:bluetooth-5-3"}, sku("a1", 1, "color:den"))
	d.Embedding = make384()
	upsert(t, idx, d)
	res, err := idx.SearchVector(context.Background(), make384(),
		map[string]string{"tag.connectivity": "bluetooth-5-3", "sku.color": "den"}, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatalf("k-NN leg rejected the attr filters: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Errorf("k-NN hits = %+v, want a", res.Hits)
	}
}

// Classifier outage (tags_pending) keeps the tags already stored; a successful
// re-classification replaces them (including with none).
func TestIT_Attr_TagsPendingKeepsStoredTags(t *testing.T) {
	idx, url, name := readyIndex(t)
	upsert(t, idx, phone("a", 10, []string{"connectivity:bluetooth-5-3"}, sku("a1", 1, "color:den")))

	pending := phone("a", 20, nil)
	pending.Title = "renamed while classifier down"
	pending.TagsPending = true
	upsert(t, idx, pending)
	src := mustSource(t, url, name, "a")
	if src["title"] != "renamed while classifier down" || src["tags_pending"] != true {
		t.Fatalf("base fields not applied: %v", src)
	}
	if tags, _ := src["facet_tags"].([]any); len(tags) != 1 {
		t.Errorf("facet_tags dropped by the outage write: %v", src["facet_tags"])
	}
	if skus, _ := src["skus"].([]any); len(skus) != 1 {
		t.Errorf("skus dropped by the outage write: %v", src["skus"])
	}

	upsert(t, idx, phone("a", 30, []string{"connectivity:wifi-6"}))
	src = mustSource(t, url, name, "a")
	if tags, _ := src["facet_tags"].([]any); len(tags) != 1 || tags[0] != "connectivity:wifi-6" {
		t.Errorf("re-classification must replace tags: %v", src["facet_tags"])
	}
	if _, has := src["skus"]; has {
		t.Errorf("re-classification with no variants must clear skus: %v", src["skus"])
	}
}
