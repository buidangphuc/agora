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

func TestRRF_CarriesStock(t *testing.T) {
	three, zero := int32(3), int32(0)
	fused := retrieval.RRF(map[string][]retrieval.Candidate{
		"lexical":  {{ListingID: "a", Stock: &three}, {ListingID: "b"}},
		"semantic": {{ListingID: "b", Stock: &zero}, {ListingID: "c"}},
	}, 60, nil)
	got := map[string]*int32{}
	for _, c := range fused {
		got[c.ListingID] = c.Stock
	}
	if got["a"] == nil || *got["a"] != 3 || got["b"] == nil || *got["b"] != 0 || got["c"] != nil {
		t.Errorf("stock not carried through RRF: a=%v b=%v c=%v", got["a"], got["b"], got["c"])
	}
}

func TestRRFTiesAreOrderedByListingIDOnEveryCall(t *testing.T) {
	// "b" and "c" each get rank 1 in one leg, and "a" and "d" rank 2: two exact ties per pair.
	legs := map[string][]retrieval.Candidate{
		"lexical":  {{ListingID: "c"}, {ListingID: "d"}},
		"semantic": {{ListingID: "b"}, {ListingID: "a"}},
	}
	for i := 0; i < 200; i++ {
		got := retrieval.RRF(legs, 60, nil)
		ids := make([]string, len(got))
		for j, c := range got {
			ids[j] = c.ListingID
		}
		if want := []string{"b", "c", "a", "d"}; !equalIDs(ids, want) {
			t.Fatalf("call %d: order %v, want %v", i, ids, want)
		}
	}
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
