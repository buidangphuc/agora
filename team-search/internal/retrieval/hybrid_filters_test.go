package retrieval_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

// osLikeIndex mimics OpenSearch: the k-NN leg reports total = number of
// neighbours returned, scores are (1+cosine)/2, and facets can be re-aggregated
// over an explicit id set.
type osLikeIndex struct {
	fakeIndex
	lexFilters, vecFilters map[string]string
	facetIDs               []string
}

func (o *osLikeIndex) Search(ctx context.Context, q string, f map[string]string, c string, a, b int64, r int32, s searchv1.SortBy, from, size int) (index.SearchResult, error) {
	o.lexFilters = f
	return o.fakeIndex.Search(ctx, q, f, c, a, b, r, s, from, size)
}

func (o *osLikeIndex) SearchVector(ctx context.Context, v []float32, f map[string]string, c string, a, b int64, r int32, s searchv1.SortBy, from, size int) (index.SearchResult, error) {
	o.vecFilters = f
	res, err := o.fakeIndex.SearchVector(ctx, v, f, c, a, b, r, s, from, size)
	res.Total = 50 // OpenSearch's knn total is k, not the number of relevant listings
	return res, err
}

func (o *osLikeIndex) FacetsForIDs(_ context.Context, ids []string, _ map[string]string) (index.Facets, error) {
	o.facetIDs = append([]string(nil), ids...)
	sort.Strings(o.facetIDs)
	return index.Facets{Categories: []index.FacetBucket{{Key: "fused", Count: int64(len(ids))}}}, nil
}

func cosScore(cos float64) float64 { return (1 + cos) / 2 }

func hybridEngine(idx index.Index) *retrieval.Engine {
	return retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{
		EnableHybridSearch: true, HybridFusionWindow: 200,
	})
}

var hybridQ = retrieval.SearchParams{Query: "q", SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID, Size: 10,
	Filters: map[string]string{"in_stock": "true", "seller_id": "s1"}, CategoryID: "c1", MinPrice: 5, MaxPrice: 9}

// Both legs receive every structured filter.
func TestEngine_FiltersReachBothLegs(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		lexHits: []index.Hit{{ListingID: "a", Score: 3}}, vecHits: []index.Hit{{ListingID: "b", Score: cosScore(0.9)}},
	}}
	if _, _, err := hybridEngine(idx).Execute(context.Background(), hybridQ); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(idx.lexFilters, hybridQ.Filters) || !reflect.DeepEqual(idx.vecFilters, hybridQ.Filters) {
		t.Errorf("filters lex=%v vec=%v, want %v on both legs", idx.lexFilters, idx.vecFilters, hybridQ.Filters)
	}
}

// Total and facets describe the fused candidates, not the k-NN leg's k.
func TestEngine_TotalAndFacetsFollowFusedSet(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		lexHits: []index.Hit{{ListingID: "a", Score: 3}},
		vecHits: []index.Hit{{ListingID: "a", Score: cosScore(0.95)}, {ListingID: "b", Score: cosScore(0.8)}},
	}}
	res, _, err := hybridEngine(idx).Execute(context.Background(), hybridQ)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Errorf("total = %d, want the 2 fused candidates", res.Total)
	}
	if !reflect.DeepEqual(idx.facetIDs, []string{"a", "b"}) {
		t.Errorf("facets aggregated over %v, want the fused ids [a b]", idx.facetIDs)
	}
	if len(res.Facets.Categories) != 1 || res.Facets.Categories[0].Key != "fused" {
		t.Errorf("facets = %+v, want the fused-set aggregation", res.Facets)
	}
}

// Fused set equals the lexical set: lexical facets are reused, no extra query.
func TestEngine_FusedEqualsLexicalReusesLexicalFacets(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		lexHits: []index.Hit{{ListingID: "a", Score: 3}}, vecHits: []index.Hit{{ListingID: "a", Score: cosScore(0.95)}},
	}}
	res, _, err := hybridEngine(idx).Execute(context.Background(), hybridQ)
	if err != nil {
		t.Fatal(err)
	}
	if idx.facetIDs != nil || res.Facets.Categories[0].Key != "cat-1" {
		t.Errorf("unexpected re-aggregation: ids=%v facets=%+v", idx.facetIDs, res.Facets)
	}
}
