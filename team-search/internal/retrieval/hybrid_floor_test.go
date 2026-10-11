package retrieval_test

import (
	"context"
	"reflect"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

func floorEngine(idx index.Index, minScore float64) *retrieval.Engine {
	return retrieval.NewEngine(idx, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{
		EnableHybridSearch: true, HybridFusionWindow: 200, SemanticMinScore: minScore,
	})
}

// The similarity floor: a semantic candidate below it is dropped; with an empty
// lexical leg that means zero hits, zero total and a non-nil facet set.
func TestEngine_SemanticFloorDropsUnrelatedCandidates(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		vecHits: []index.Hit{{ListingID: "n1", Score: cosScore(0.05)}, {ListingID: "n2", Score: cosScore(-0.04)}},
	}}
	res, degraded, err := floorEngine(idx, 0.6).Execute(context.Background(), hybridQ)
	if err != nil || degraded {
		t.Fatalf("err=%v degraded=%v", err, degraded)
	}
	if len(res.Hits) != 0 || res.Total != 0 {
		t.Errorf("hits=%v total=%d, want none: every neighbour is below the floor", res.Hits, res.Total)
	}
}

func TestEngine_SemanticFloorKeepsRelatedAndPartiallyDrops(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		vecHits: []index.Hit{{ListingID: "syn", Score: cosScore(1)}, {ListingID: "noise", Score: cosScore(0.02)}},
	}}
	res, _, err := floorEngine(idx, 0.6).Execute(context.Background(), hybridQ)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].ListingID != "syn" || res.Total != 1 {
		t.Errorf("hits=%v total=%d, want only syn", res.Hits, res.Total)
	}
	if !reflect.DeepEqual(idx.facetIDs, []string{"syn"}) {
		t.Errorf("facets over %v, want [syn]", idx.facetIDs)
	}
}

// Lexical hits survive when every semantic candidate is under the floor.
func TestEngine_SemanticFloorLeavesLexicalHits(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		lexHits: []index.Hit{{ListingID: "a", Score: 3}}, vecHits: []index.Hit{{ListingID: "n", Score: cosScore(0)}},
	}}
	res, _, err := floorEngine(idx, 0.6).Execute(context.Background(), hybridQ)
	if err != nil || len(res.Hits) != 1 || res.Hits[0].ListingID != "a" || res.Total != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestEngine_SemanticFloorDisabledKeepsNeighbours(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{vecHits: []index.Hit{{ListingID: "n", Score: cosScore(-0.5)}}}}
	res, _, err := floorEngine(idx, -1).Execute(context.Background(), hybridQ)
	if err != nil || len(res.Hits) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestEngine_SemanticModeAppliesFloor(t *testing.T) {
	idx := &osLikeIndex{fakeIndex: fakeIndex{
		vecHits: []index.Hit{{ListingID: "syn", Score: cosScore(1)}, {ListingID: "noise", Score: cosScore(0.02)}},
	}}
	p := hybridQ
	p.SearchMode = searchv1.SearchMode_SEARCH_MODE_SEMANTIC
	res, _, err := floorEngine(idx, 0.6).Execute(context.Background(), p)
	if err != nil || len(res.Hits) != 1 || res.Total != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
