package taxonomy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/team-search/internal/taxonomy"
)

func serve(t *testing.T, status int, resp string, got *map[string]any, path *string) *taxonomy.HTTPClassifier {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*path = r.URL.Path
		_ = json.Unmarshal(b, got)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return taxonomy.NewHTTPClassifier(srv.URL, time.Second)
}

func TestClassify_ListingWithVariantsUsesHierarchyEndpoint(t *testing.T) {
	var got map[string]any
	var path string
	c := serve(t, 200, `{
	  "spu_canonical_tags": [
	    {"slug": "chong-nuoc-ipx7", "facet_group": "feature"},
	    {"slug": "bluetooth-5-3", "facet_group": "connectivity"},
	    {"slug": "Bad Slug", "facet_group": "feature"},
	    {"slug": "bluetooth-5-3", "facet_group": "connectivity"}],
	  "sku_results": [
	    {"variant_facets": {"color": "titan-tu-nhien", "capacity": "256gb"}},
	    {"variant_facets": {"color": "xanh-navy", "capacity": "512gb", "bad": "Not A Slug"}}]
	}`, &got, &path)

	res, err := c.Classify(context.Background(), taxonomy.Listing{
		Title: "iPhone 15 Pro Max", CategoryID: "cat-phones",
		Variants: []taxonomy.Variant{
			{ID: "v1", Name: "Titan Tự Nhiên / 256GB", Price: 30000000, Stock: 5},
			{ID: "v2", Name: "Xanh Navy / 512GB", Price: 34000000, Stock: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/ai/tags/classify-sku-hierarchy" {
		t.Errorf("path = %s", path)
	}
	if got["spu_title"] != "iPhone 15 Pro Max" || len(got["variants"].([]any)) != 2 {
		t.Errorf("request body = %v", got)
	}
	if want := []string{"connectivity:bluetooth-5-3", "feature:chong-nuoc-ipx7"}; !reflect.DeepEqual(res.FacetTags, want) {
		t.Errorf("FacetTags = %v, want %v (sorted, deduped, malformed dropped)", res.FacetTags, want)
	}
	if len(res.SKUs) != 2 {
		t.Fatalf("SKUs = %+v", res.SKUs)
	}
	if want := []string{"capacity:256gb", "color:titan-tu-nhien"}; !reflect.DeepEqual(res.SKUs[0].Attrs, want) || !res.SKUs[0].InStock {
		t.Errorf("sku0 = %+v", res.SKUs[0])
	}
	if want := []string{"capacity:512gb", "color:xanh-navy"}; !reflect.DeepEqual(res.SKUs[1].Attrs, want) || res.SKUs[1].InStock {
		t.Errorf("sku1 = %+v (sold out must not be in stock)", res.SKUs[1])
	}
}

func TestClassify_ListingWithoutVariantsUsesSpuEndpoint(t *testing.T) {
	var got map[string]any
	var path string
	c := serve(t, 200, `{"canonical_tags":[{"slug":"bluetooth-5-3","facet_group":"connectivity"}]}`, &got, &path)
	res, err := c.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe Bluetooth 5.3"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/ai/tags/classify" || got["title"] != "Tai nghe Bluetooth 5.3" {
		t.Errorf("path=%s body=%v", path, got)
	}
	if !reflect.DeepEqual(res.FacetTags, []string{"connectivity:bluetooth-5-3"}) || len(res.SKUs) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestClassify_ErrorsAreErrorsNotEmptyAnswers(t *testing.T) {
	var got map[string]any
	var path string
	c := serve(t, 503, `down`, &got, &path)
	if _, err := c.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("a 503 must be an error so the indexer marks tags_pending")
	}
	c2 := serve(t, 200, `not json`, &got, &path)
	if _, err := c2.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("undecodable body must be an error")
	}
	unreachable := taxonomy.NewHTTPClassifier("http://127.0.0.1:1", 200*time.Millisecond)
	if _, err := unreachable.Classify(context.Background(), taxonomy.Listing{Title: "Tai nghe"}); err == nil {
		t.Error("unreachable team-ai must be an error")
	}
}

func TestClassify_TooShortTitleIsNothingToClassify(t *testing.T) {
	var got map[string]any
	var path string
	c := serve(t, 500, ``, &got, &path)
	res, err := c.Classify(context.Background(), taxonomy.Listing{Title: " a "})
	if err != nil || len(res.FacetTags) != 0 || path != "" {
		t.Errorf("res=%+v err=%v path=%q: must not call team-ai", res, err, path)
	}
}
