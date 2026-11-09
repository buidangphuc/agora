package retrieval_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

type sameOrderRerank struct{}

func (sameOrderRerank) Rerank(_ context.Context, _ string, docs []retrieval.RerankDoc) ([]string, error) {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}
	return out, nil
}

func hybridParams() retrieval.SearchParams {
	return retrieval.SearchParams{Query: "q", SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID, Size: 10}
}

func hybridCfg(rerank bool) config.Retrieval {
	return config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200, EnableReranker: rerank}
}

func TestSearchAttribution_HybridHitsKeepSoldRatingReviewSeller(t *testing.T) {
	lex := []index.Hit{
		{ListingID: "a", Score: 5, SoldCount: 12, RatingAvg: 4.5, ReviewCount: 30, SellerID: "s-a"},
		{ListingID: "b", Score: 4},
	}
	vec := []index.Hit{
		{ListingID: "b", Score: 0.9, SoldCount: 7, RatingAvg: 3.5, ReviewCount: 9, SellerID: "s-b"},
		{ListingID: "c", Score: 0.8, SoldCount: 1, RatingAvg: 2.0, ReviewCount: 2, SellerID: "s-c"},
	}
	e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, nil, hybridCfg(false))
	res, mode, _, err := e.Execute(context.Background(), hybridParams())
	if err != nil || string(mode) != "hybrid" {
		t.Fatalf("err=%v mode=%q", err, mode)
	}
	want := map[string]index.Hit{
		"a": {SoldCount: 12, RatingAvg: 4.5, ReviewCount: 30, SellerID: "s-a"},
		"b": {SoldCount: 7, RatingAvg: 3.5, ReviewCount: 9, SellerID: "s-b"},
		"c": {SoldCount: 1, RatingAvg: 2.0, ReviewCount: 2, SellerID: "s-c"},
	}
	if len(res.Hits) != len(want) {
		t.Fatalf("hits = %+v", res.Hits)
	}
	for _, h := range res.Hits {
		w := want[h.ListingID]
		if h.SoldCount != w.SoldCount || h.RatingAvg != w.RatingAvg || h.ReviewCount != w.ReviewCount || h.SellerID != w.SellerID {
			t.Errorf("%s = sold %d rating %v reviews %d seller %q, want %+v", h.ListingID, h.SoldCount, h.RatingAvg, h.ReviewCount, h.SellerID, w)
		}
	}
}

func TestSearchAttribution_HybridHitsKeepFieldsAfterRerank(t *testing.T) {
	lex := []index.Hit{{ListingID: "a", Score: 5, SoldCount: 12, RatingAvg: 4.5, ReviewCount: 30, SellerID: "s-a"}}
	vec := []index.Hit{{ListingID: "b", Score: 0.9, SoldCount: 7, RatingAvg: 3.5, ReviewCount: 9, SellerID: "s-b"}}
	e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, reverseRerank{}, hybridCfg(true))
	res, _, _, err := e.Execute(context.Background(), hybridParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.SoldCount == 0 || h.RatingAvg == 0 || h.ReviewCount == 0 || h.SellerID == "" {
			t.Errorf("%s lost fields: %+v", h.ListingID, h)
		}
	}
}

func TestSearchAttribution_RerankReportedOnlyWhenOrderChanges(t *testing.T) {
	lex := []index.Hit{{ListingID: "a", Score: 5}, {ListingID: "b", Score: 4}, {ListingID: "c", Score: 3}}
	vec := []index.Hit{{ListingID: "a", Score: 0.9}, {ListingID: "b", Score: 0.8}, {ListingID: "c", Score: 0.7}}
	cases := []struct {
		name string
		rr   retrieval.RerankClient
		want bool
	}{
		{"same order is not rerank", sameOrderRerank{}, false},
		{"reordering is rerank", reverseRerank{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, c.rr, hybridCfg(true))
			_, _, reranked, err := e.Execute(context.Background(), hybridParams())
			if err != nil {
				t.Fatal(err)
			}
			if reranked != c.want {
				t.Errorf("reranked = %v, want %v", reranked, c.want)
			}
		})
	}
}
