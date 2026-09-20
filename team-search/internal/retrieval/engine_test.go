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

type fakeIndex struct {
	lexHits    []index.Hit
	vecHits    []index.Hit
	lexErr     error
	vecErr     error
	lexCalled  bool
	vecCalled  bool
	lastVector []float32
}

func (f *fakeIndex) EnsureIndex(ctx context.Context) error                  { return nil }
func (f *fakeIndex) Upsert(ctx context.Context, doc index.ListingDoc) error { return nil }
func (f *fakeIndex) PartialUpdate(ctx context.Context, id string, p map[string]interface{}) error {
	return nil
}
func (f *fakeIndex) Delete(ctx context.Context, id string) error { return nil }
func (f *fakeIndex) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	return nil, nil
}

func (f *fakeIndex) Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	f.lexCalled = true
	if f.lexErr != nil {
		return index.SearchResult{}, f.lexErr
	}
	return index.SearchResult{
		Hits:  f.lexHits,
		Total: int64(len(f.lexHits)),
		Facets: index.Facets{
			Categories: []index.FacetBucket{{Key: "cat-1", Count: int64(len(f.lexHits))}},
		},
	}, nil
}

func (f *fakeIndex) SearchVector(ctx context.Context, vector []float32, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	f.vecCalled = true
	f.lastVector = vector
	if f.vecErr != nil {
		return index.SearchResult{}, f.vecErr
	}
	return index.SearchResult{
		Hits:  f.vecHits,
		Total: int64(len(f.vecHits)),
		Facets: index.Facets{
			Categories: []index.FacetBucket{{Key: "cat-1", Count: int64(len(f.vecHits))}},
		},
	}, nil
}

func TestEngine_HybridConcurrentRetrievalAndFusion(t *testing.T) {
	idx := &fakeIndex{
		lexHits: []index.Hit{{ListingID: "item-1", Score: 5.0}, {ListingID: "item-2", Score: 3.0}},
		vecHits: []index.Hit{{ListingID: "item-2", Score: 0.95}, {ListingID: "item-3", Score: 0.85}},
	}
	embed := &retrieval.MockEmbedClient{Dim: 4}
	cfg := config.Retrieval{
		EnableHybridSearch: true,
		HybridRRFK:         60,
		HybridFusionWindow: 200,
		LexicalWeight:      1.0,
		SemanticWeight:     1.0,
	}

	engine := retrieval.NewEngine(idx, embed, nil, cfg)
	res, isDegraded, err := engine.Execute(context.Background(), retrieval.SearchParams{
		Query:      "shoes",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID,
		From:       0,
		Size:       10,
	})

	if err != nil {
		t.Fatalf("Engine execute failed: %v", err)
	}
	if isDegraded {
		t.Errorf("expected isDegraded to be false")
	}
	if !idx.lexCalled || !idx.vecCalled {
		t.Errorf("expected both lexical and vector strategies to be called: lex=%v, vec=%v", idx.lexCalled, idx.vecCalled)
	}

	// item-2 appears in both, so it should rank 1st in RRF
	if len(res.Hits) != 3 {
		t.Fatalf("expected 3 fused hits, got %d", len(res.Hits))
	}
	if res.Hits[0].ListingID != "item-2" {
		t.Errorf("expected item-2 to be top ranked, got %s", res.Hits[0].ListingID)
	}
}

func TestEngine_FailOpenWhenEmbeddingFails(t *testing.T) {
	idx := &fakeIndex{
		lexHits: []index.Hit{{ListingID: "lex-item-1", Score: 5.0}},
	}
	embed := &retrieval.MockEmbedClient{Err: errors.New("modelserve 503 unavailable")}
	cfg := config.Retrieval{
		EnableHybridSearch: true,
		HybridRRFK:         60,
		HybridFusionWindow: 200,
	}

	engine := retrieval.NewEngine(idx, embed, nil, cfg)
	res, isDegraded, err := engine.Execute(context.Background(), retrieval.SearchParams{
		Query:      "shoes",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID,
		From:       0,
		Size:       10,
	})

	if err != nil {
		t.Fatalf("expected fail-open success, got error: %v", err)
	}
	if !isDegraded {
		t.Errorf("expected isDegraded = true")
	}
	if len(res.Hits) != 1 || res.Hits[0].ListingID != "lex-item-1" {
		t.Errorf("expected fallback to lexical results, got: %+v", res.Hits)
	}
}

func TestEngine_DeepPaginationFallsBackToBM25(t *testing.T) {
	idx := &fakeIndex{
		lexHits: []index.Hit{{ListingID: "deep-item", Score: 1.0}},
	}
	embed := &retrieval.MockEmbedClient{Dim: 4}
	cfg := config.Retrieval{
		EnableHybridSearch: true,
		HybridFusionWindow: 200,
	}

	engine := retrieval.NewEngine(idx, embed, nil, cfg)
	// From = 250 > HybridFusionWindow (200)
	res, _, err := engine.Execute(context.Background(), retrieval.SearchParams{
		Query:      "shoes",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID,
		From:       250,
		Size:       10,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !idx.lexCalled {
		t.Errorf("expected lexical to be called")
	}
	if idx.vecCalled {
		t.Errorf("expected vector strategy to be skipped on deep pagination (>200)")
	}
	if len(res.Hits) != 1 || res.Hits[0].ListingID != "deep-item" {
		t.Errorf("unexpected hits: %+v", res.Hits)
	}
}

func TestEngine_ExplicitLexicalMode(t *testing.T) {
	idx := &fakeIndex{
		lexHits: []index.Hit{{ListingID: "lex-only", Score: 2.0}},
	}
	embed := &retrieval.MockEmbedClient{Dim: 4}
	cfg := config.Retrieval{EnableHybridSearch: true}

	engine := retrieval.NewEngine(idx, embed, nil, cfg)
	res, _, err := engine.Execute(context.Background(), retrieval.SearchParams{
		Query:      "shoes",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL,
		From:       0,
		Size:       10,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !idx.lexCalled || idx.vecCalled {
		t.Errorf("expected only lexical called, got lex=%v, vec=%v", idx.lexCalled, idx.vecCalled)
	}
	if len(res.Hits) != 1 || res.Hits[0].ListingID != "lex-only" {
		t.Errorf("unexpected hits: %+v", res.Hits)
	}
}
