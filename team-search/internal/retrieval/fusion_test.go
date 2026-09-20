package retrieval_test

import (
	"testing"

	"github.com/buidangphuc/team-search/internal/retrieval"
)

func TestRRF_BoostsItemsAppearingInMultipleLists(t *testing.T) {
	// Item "A" appears rank 1 in lexical, rank 1 in semantic.
	// Item "B" appears rank 2 in lexical, not in semantic.
	// Item "C" appears rank 2 in semantic, not in lexical.
	lexical := []retrieval.Candidate{
		{ListingID: "item-A", Score: 10.0},
		{ListingID: "item-B", Score: 8.0},
	}
	semantic := []retrieval.Candidate{
		{ListingID: "item-A", Score: 0.95},
		{ListingID: "item-C", Score: 0.90},
	}

	strategyMap := map[string][]retrieval.Candidate{
		"lexical":  lexical,
		"semantic": semantic,
	}

	fused := retrieval.RRF(strategyMap, 60, nil)

	if len(fused) != 3 {
		t.Fatalf("expected 3 fused items, got %d", len(fused))
	}

	if fused[0].ListingID != "item-A" {
		t.Errorf("expected item-A to rank 1st due to dual-strategy boost, got %s", fused[0].ListingID)
	}

	// Score of item-A: 1/(60+1) + 1/(60+1) = 2/61 ≈ 0.03278
	expectedScoreA := (1.0 / 61.0) + (1.0 / 61.0)
	if diff := fused[0].Score - expectedScoreA; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("expected item-A score %f, got %f", expectedScoreA, fused[0].Score)
	}
}

func TestRRF_WeightsScaleStrategyInfluence(t *testing.T) {
	// Lexical has X at rank 1.
	// Semantic has Y at rank 1.
	// If semantic weight is 3.0 and lexical is 1.0, Y must beat X.
	lexical := []retrieval.Candidate{
		{ListingID: "item-X", Score: 10.0},
	}
	semantic := []retrieval.Candidate{
		{ListingID: "item-Y", Score: 0.9},
	}

	strategyMap := map[string][]retrieval.Candidate{
		"lexical":  lexical,
		"semantic": semantic,
	}
	weights := map[string]float64{
		"lexical":  1.0,
		"semantic": 3.0,
	}

	fused := retrieval.RRF(strategyMap, 60, weights)

	if len(fused) != 2 {
		t.Fatalf("expected 2 items, got %d", len(fused))
	}
	if fused[0].ListingID != "item-Y" {
		t.Errorf("expected item-Y to rank 1st with 3x semantic weight, got %s", fused[0].ListingID)
	}
}
