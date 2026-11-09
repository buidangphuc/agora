package index_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

func vectorRatingFixture(t *testing.T) *index.OpenSearchIndex {
	t.Helper()
	idx, _, _ := readyIndex(t)
	for _, c := range []struct {
		id  string
		avg float64
	}{{"hi", 4.5}, {"mid", 3.5}, {"low", 3.2}, {"lo", 2}} {
		d := liveDoc(c.id, "ibex "+c.id, 100)
		d.Embedding = make384()
		upsert(t, idx, d)
		updRating(t, idx, c.id, c.avg, 1, 100)
	}
	return idx
}

func ratingCounts(t *testing.T, got []index.FacetBucket) (four, three int64) {
	t.Helper()
	if len(got) != 2 || got[0].Key != "4" || got[1].Key != "3" {
		t.Fatalf("ratings buckets = %+v, want keys 4 then 3", got)
	}
	return got[0].Count, got[1].Count
}

func TestIT_BR18a_SearchVectorRatingsFacetIgnoresItsOwnFilter(t *testing.T) {
	idx := vectorRatingFixture(t)
	res, err := idx.SearchVector(context.Background(), make384(), nil, "", 0, 0, 4, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, res, "hi")
	four, three := ratingCounts(t, res.Facets.Ratings)
	if four != 1 || three != 3 {
		t.Errorf("ratings = 4:%d 3:%d, want 4:1 3:3 over the unfiltered set", four, three)
	}
	if len(res.Facets.Categories) != 1 || res.Facets.Categories[0].Count != 1 {
		t.Errorf("categories must still honour min_rating: %+v", res.Facets.Categories)
	}
}

func TestIT_BR18a_EngineSemanticAndHybridRatingsFacetIgnoreMinRating(t *testing.T) {
	idx := vectorRatingFixture(t)
	cfg := config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200}
	for _, mode := range []searchv1.SearchMode{searchv1.SearchMode_SEARCH_MODE_SEMANTIC, searchv1.SearchMode_SEARCH_MODE_HYBRID} {
		e := retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 384}, nil, cfg)
		res, _, _, err := e.Execute(context.Background(), retrieval.SearchParams{Query: "ibex", MinRating: 4, SearchMode: mode, Size: 10})
		if err != nil {
			t.Fatalf("%v: %v", mode, err)
		}
		wantIDs(t, res, "hi")
		four, three := ratingCounts(t, res.Facets.Ratings)
		if four != 1 || three != 3 {
			t.Errorf("%v: ratings = 4:%d 3:%d, want 4:1 3:3 (bucket 3 must exceed bucket 4)", mode, four, three)
		}
		if len(res.Facets.Categories) != 1 || res.Facets.Categories[0].Count != 1 {
			t.Errorf("%v: categories must still honour min_rating: %+v", mode, res.Facets.Categories)
		}
	}
}
