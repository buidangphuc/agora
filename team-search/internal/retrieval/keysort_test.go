package retrieval_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

// D8: a hybrid request (explicit, or defaulted for a non-empty query) with a key
// sort is served by the lexical leg with that sort and page, never fused; the
// k-NN leg (and so RRF, which needs both) is not run.
func TestEngine_KeySortBypassesFusion(t *testing.T) {
	cfg := config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200}
	lex := []index.Hit{{ListingID: "z"}, {ListingID: "a"}}
	vec := []index.Hit{{ListingID: "a"}, {ListingID: "z"}}
	for _, sortBy := range []searchv1.SortBy{searchv1.SortBy_SORT_BY_NEWEST, searchv1.SortBy_SORT_BY_PRICE_ASC, searchv1.SortBy_SORT_BY_PRICE_DESC} {
		for _, mode := range []searchv1.SearchMode{searchv1.SearchMode_SEARCH_MODE_HYBRID, searchv1.SearchMode_SEARCH_MODE_UNSPECIFIED} {
			idx := &fakeIndex{lexHits: lex, vecHits: vec}
			e := retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, cfg)
			res, degraded, err := e.Execute(context.Background(), retrieval.SearchParams{Query: "q", SortBy: sortBy, SearchMode: mode, From: 20, Size: 5})
			if err != nil || degraded {
				t.Fatalf("%v/%v: err=%v degraded=%v", sortBy, mode, err, degraded)
			}
			if idx.vecCalled {
				t.Errorf("%v/%v: k-NN leg ran, so the result was fused", sortBy, mode)
			}
			if !idx.lexCalled || idx.lexSort != sortBy || idx.lexFrom != 20 || idx.lexSize != 5 {
				t.Errorf("%v/%v: lexical leg got sort=%v from=%d size=%d", sortBy, mode, idx.lexSort, idx.lexFrom, idx.lexSize)
			}
			if len(res.Hits) != 2 || res.Hits[0].ListingID != "z" {
				t.Errorf("%v/%v: lexical key order not kept: %+v", sortBy, mode, res.Hits)
			}
		}
	}
}

func TestEngine_RelevanceSortStillFuses(t *testing.T) {
	cfg := config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200}
	for _, sortBy := range []searchv1.SortBy{searchv1.SortBy_SORT_BY_UNSPECIFIED, searchv1.SortBy_SORT_BY_RELEVANCE} {
		idx := &fakeIndex{lexHits: []index.Hit{{ListingID: "a"}}, vecHits: []index.Hit{{ListingID: "b", Score: 0.9}}}
		e := retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, cfg)
		res, _, err := e.Execute(context.Background(), retrieval.SearchParams{Query: "q", SortBy: sortBy, Size: 10})
		if err != nil {
			t.Fatal(err)
		}
		if !idx.vecCalled || !idx.lexCalled || len(res.Hits) != 2 {
			t.Errorf("%v: expected fusion of both legs, lex=%v vec=%v hits=%+v", sortBy, idx.lexCalled, idx.vecCalled, res.Hits)
		}
	}
}
