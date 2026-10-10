package retrieval_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

// slowIndex makes each retrieval leg take `delay`, so a sequential engine needs 2*delay.
type slowIndex struct {
	fakeIndex
	delay time.Duration
}

func (s *slowIndex) Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	time.Sleep(s.delay)
	return s.fakeIndex.Search(ctx, query, filters, categoryID, minPrice, maxPrice, minRating, sortBy, from, size)
}

func (s *slowIndex) SearchVector(ctx context.Context, vector []float32, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	time.Sleep(s.delay)
	return s.fakeIndex.SearchVector(ctx, vector, filters, categoryID, minPrice, maxPrice, minRating, sortBy, from, size)
}

// Spec scenario "Multi-strategy retrieval executes strategies concurrently"
// (change add-hybrid-retrieval-platform): the lexical and semantic stages overlap in time.
func TestEngine_StrategiesRunConcurrently(t *testing.T) {
	const delay = 250 * time.Millisecond
	idx := &slowIndex{
		fakeIndex: fakeIndex{
			lexHits: []index.Hit{{ListingID: "item-1", Score: 5}},
			vecHits: []index.Hit{{ListingID: "item-2", Score: 0.9}},
		},
		delay: delay,
	}
	engine := retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{
		EnableHybridSearch: true,
		HybridRRFK:         60,
		HybridFusionWindow: 200,
	})

	started := time.Now()
	res, degraded, err := engine.Execute(context.Background(), retrieval.SearchParams{
		Query:      "shoes",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_HYBRID,
		Size:       10,
	})
	elapsed := time.Since(started)

	if err != nil || degraded {
		t.Fatalf("execute: err=%v degraded=%v", err, degraded)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("expected both legs to contribute a hit, got %d", len(res.Hits))
	}
	if elapsed >= 2*delay {
		t.Errorf("legs ran one after the other: took %v, each leg takes %v", elapsed, delay)
	}
}

// Spec scenario "RRF constant k stabilizes rank position weights": with k=60 the fused score
// falls smoothly with rank (no outlier), where a tiny k lets the top rank dominate.
func TestRRF_ConstantKFlattensRankScores(t *testing.T) {
	list := make([]retrieval.Candidate, 0, 10)
	for _, id := range []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8", "r9", "r10"} {
		list = append(list, retrieval.Candidate{ListingID: id})
	}
	byID := func(k int) map[string]float64 {
		out := map[string]float64{}
		for _, c := range retrieval.RRF(map[string][]retrieval.Candidate{"lexical": list}, k, nil) {
			out[c.ListingID] = c.Score
		}
		return out
	}

	s60 := byID(60)
	for i := 1; i < 10; i++ {
		prev, next := s60["r"+strconv.Itoa(i)], s60["r"+strconv.Itoa(i+1)]
		if !(prev > next) {
			t.Errorf("k=60 scores must decrease with rank: r%d=%f r%d=%f", i, prev, i+1, next)
		}
	}
	if want := 1.0 / 61.0; s60["r1"] < want-1e-9 || s60["r1"] > want+1e-9 {
		t.Errorf("rank 1 at k=60 must score 1/61, got %f", s60["r1"])
	}
	// no outlier: the best rank is within 15% of the tenth at k=60, but 5x at k=1
	if ratio := s60["r1"] / s60["r10"]; ratio > 1.15 {
		t.Errorf("k=60 ratio rank1/rank10 = %f, expected a smooth curve under 1.15", ratio)
	}
	if ratio := byID(1)["r1"] / byID(1)["r10"]; ratio < 5 {
		t.Errorf("k=1 ratio rank1/rank10 = %f, expected the top rank to dominate", ratio)
	}
	// a non-positive k falls back to 60
	if byID(0)["r1"] != s60["r1"] {
		t.Errorf("k=0 must default to 60")
	}
}
