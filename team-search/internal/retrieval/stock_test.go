package retrieval_test

import (
	"context"
	"errors"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

func i32(v int32) *int32 { return &v }

// reverseRerank reorders the candidates back to front.
type reverseRerank struct{}

func (reverseRerank) Rerank(_ context.Context, _ string, docs []retrieval.RerankDoc) ([]string, error) {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[len(docs)-1-i] = d.ID
	}
	return out, nil
}

func stockByID(hits []index.Hit) map[string]*int32 {
	m := make(map[string]*int32, len(hits))
	for _, h := range hits {
		m[h.ListingID] = h.Stock
	}
	return m
}

func wantStocks(t *testing.T, name string, hits []index.Hit, want map[string]*int32) {
	t.Helper()
	got := stockByID(hits)
	if len(got) != len(want) {
		t.Fatalf("%s: hits %+v, want ids %v", name, hits, want)
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Fatalf("%s: missing hit %s", name, id)
		}
		if (g == nil) != (w == nil) || (g != nil && *g != *w) {
			t.Errorf("%s: %s stock = %v, want %v", name, id, g, w)
		}
	}
}

// D3: every path out of the engine keeps each hit's stock, including its
// absence (nil) and a sold-out 0.
func TestEngine_HitsKeepStockOnEveryPath(t *testing.T) {
	lex := []index.Hit{{ListingID: "a", Score: 5, Stock: i32(3)}, {ListingID: "b", Score: 4, Stock: i32(0)}}
	vec := []index.Hit{{ListingID: "b", Score: 0.9, Stock: i32(0)}, {ListingID: "c", Score: 0.8}}
	hybrid := retrieval.SearchParams{Query: "q", SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID, Size: 10}
	cfg := config.Retrieval{EnableHybridSearch: true, HybridFusionWindow: 200}

	t.Run("fused", func(t *testing.T) {
		e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, nil, cfg)
		res, _, err := e.Execute(context.Background(), hybrid)
		if err != nil {
			t.Fatal(err)
		}
		wantStocks(t, "fused", res.Hits, map[string]*int32{"a": i32(3), "b": i32(0), "c": nil})
	})
	t.Run("reranked", func(t *testing.T) {
		rc := cfg
		rc.EnableReranker = true
		e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, reverseRerank{}, rc)
		res, _, err := e.Execute(context.Background(), hybrid)
		if err != nil {
			t.Fatal(err)
		}
		wantStocks(t, "reranked", res.Hits, map[string]*int32{"a": i32(3), "b": i32(0), "c": nil})
	})
	t.Run("lexical fallback", func(t *testing.T) {
		e := retrieval.NewEngine(&fakeIndex{lexHits: lex, vecErr: errors.New("down")}, &retrieval.MockEmbedClient{Dim: 4}, nil, cfg)
		res, degraded, err := e.Execute(context.Background(), hybrid)
		if err != nil || !degraded {
			t.Fatalf("err=%v degraded=%v", err, degraded)
		}
		wantStocks(t, "lexical fallback", res.Hits, map[string]*int32{"a": i32(3), "b": i32(0)})
	})
	t.Run("semantic fallback", func(t *testing.T) {
		e := retrieval.NewEngine(&fakeIndex{lexErr: errors.New("down"), vecHits: vec}, &retrieval.MockEmbedClient{Dim: 4}, nil, cfg)
		res, degraded, err := e.Execute(context.Background(), hybrid)
		if err != nil || !degraded {
			t.Fatalf("err=%v degraded=%v", err, degraded)
		}
		wantStocks(t, "semantic fallback", res.Hits, map[string]*int32{"b": i32(0), "c": nil})
	})
}
