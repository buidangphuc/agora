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

type failingEmbed struct{}

func (failingEmbed) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed down")
}

func (failingEmbed) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embed down")
}

type failingRerank struct{}

func (failingRerank) Rerank(context.Context, string, []retrieval.RerankDoc) ([]string, error) {
	return nil, errors.New("rerank down")
}

func attributionIndex() *fakeIndex {
	return &fakeIndex{
		lexHits: []index.Hit{{ListingID: "item-1", Score: 5}, {ListingID: "item-2", Score: 3}},
		vecHits: []index.Hit{{ListingID: "item-2", Score: 0.95}, {ListingID: "item-3", Score: 0.9}},
	}
}

func attributionCfg(rerank bool) config.Retrieval {
	return config.Retrieval{
		EnableHybridSearch: true,
		EnableReranker:     rerank,
		HybridRRFK:         60,
		HybridFusionWindow: 200,
		LexicalWeight:      1,
		SemanticWeight:     1,
	}
}

func TestSearchAttributionEngineReportsEffectiveMode(t *testing.T) {
	hybrid := searchv1.SearchMode_SEARCH_MODE_HYBRID
	cases := []struct {
		name         string
		idx          *fakeIndex
		embed        retrieval.EmbedClient
		rerank       retrieval.RerankClient
		cfg          config.Retrieval
		params       retrieval.SearchParams
		wantMode     string
		wantReranked bool
	}{
		{
			name: "A hybrid search reports hybrid",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "hybrid",
		},
		{
			name: "The reranker is reported",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "hybrid", wantReranked: true,
		},
		{
			name: "A reranker failure is not reported as rerank",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: failingRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "hybrid",
		},
		{
			name: "A configured reranker that is disabled is not reported",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "hybrid",
		},
		{
			name: "A fallback page never carries rerank",
			idx:  attributionIndex(), embed: failingEmbed{}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Hybrid with a failed semantic leg reports lexical",
			idx: func() *fakeIndex {
				f := attributionIndex()
				f.vecErr = errors.New("knn down")
				return f
			}(),
			embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Hybrid with nothing above the semantic floor is answered by lexical",
			idx:  &fakeIndex{lexHits: attributionIndex().lexHits}, embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Hybrid with an empty lexical leg is answered by semantic",
			idx:  &fakeIndex{vecHits: attributionIndex().vecHits}, embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, Size: 10},
			wantMode: "semantic",
		},
		{
			name: "A lexical request reports lexical",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "A semantic request reports semantic",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC, Size: 10},
			wantMode: "semantic",
		},
		{
			name: "Semantic fallback reports lexical when the embedding fails",
			idx:  attributionIndex(), embed: failingEmbed{}, cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Semantic fallback reports lexical when the vector search fails",
			idx: func() *fakeIndex {
				f := attributionIndex()
				f.vecErr = errors.New("knn down")
				return f
			}(),
			embed: &retrieval.MockEmbedClient{Dim: 4}, cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Semantic fallback reports lexical without an embed client",
			idx:  attributionIndex(), cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "A key sort in hybrid mode reports lexical",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, SortBy: searchv1.SortBy_SORT_BY_PRICE_ASC, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "Deep paging beyond the fusion window reports lexical",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "shoes", SearchMode: hybrid, From: 200, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "An empty query reports lexical",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, rerank: reverseRerank{}, cfg: attributionCfg(true),
			params:   retrieval.SearchParams{Query: "", SearchMode: hybrid, Size: 10},
			wantMode: "lexical",
		},
		{
			name: "The unspecified mode resolves to the mode that ran (hybrid)",
			idx:  attributionIndex(), embed: &retrieval.MockEmbedClient{Dim: 4}, cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", Size: 10},
			wantMode: "hybrid",
		},
		{
			name: "The unspecified mode resolves to the mode that ran (lexical without embed client)",
			idx:  attributionIndex(), cfg: attributionCfg(false),
			params:   retrieval.SearchParams{Query: "shoes", Size: 10},
			wantMode: "lexical",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := retrieval.NewEngine(c.idx, c.embed, c.rerank, c.cfg)
			_, mode, reranked, err := e.Execute(context.Background(), c.params)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if string(mode) != c.wantMode {
				t.Errorf("mode = %q, want %q", mode, c.wantMode)
			}
			if reranked != c.wantReranked {
				t.Errorf("reranked = %v, want %v", reranked, c.wantReranked)
			}
		})
	}
}

func TestSearchAttributionEngineErrorReportsNoRerank(t *testing.T) {
	idx := &fakeIndex{lexErr: errors.New("index down"), vecErr: errors.New("knn down")}
	e := retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, reverseRerank{}, attributionCfg(true))
	_, _, reranked, err := e.Execute(context.Background(), retrieval.SearchParams{
		Query: "shoes", SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID, Size: 10,
	})
	if err == nil {
		t.Fatal("expected the index failure")
	}
	if reranked {
		t.Error("reranked = true on a failed search")
	}
}
