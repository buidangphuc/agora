package handler_test

import (
	"context"
	"testing"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/repository"
)

// stockIndex returns hits with present, zero and absent stock.
type stockIndex struct{ mockIndex }

func (s *stockIndex) Search(context.Context, string, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	eight, zero := int32(8), int32(0)
	return index.SearchResult{Total: 3, Hits: []index.Hit{
		{ListingID: "eight", Stock: &eight}, {ListingID: "zero", Stock: &zero}, {ListingID: "unknown"},
	}}, nil
}

func checkWireStock(t *testing.T, rpc string, hits []*searchv1.SearchHit) {
	t.Helper()
	if len(hits) != 3 {
		t.Fatalf("%s: %d hits", rpc, len(hits))
	}
	if hits[0].Stock == nil || hits[0].GetStock() != 8 {
		t.Errorf("%s: present stock lost: %v", rpc, hits[0])
	}
	if hits[1].Stock == nil || hits[1].GetStock() != 0 {
		t.Errorf("%s: zero stock must be present as 0: %v", rpc, hits[1])
	}
	if hits[2].Stock != nil {
		t.Errorf("%s: unknown stock must be absent, got %d", rpc, hits[2].GetStock())
	}
}

// D3: SearchHit.stock keeps presence on SearchListings and RunSavedSearch.
func TestSearchHitStock_PresentZeroAbsent(t *testing.T) {
	h := handler.NewSearchHandler(&stockIndex{}, repository.NewInMemorySavedSearchRepository())
	ctx := principalCtx("B", "user", "search:read", "search:write")
	res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	checkWireStock(t, "SearchListings", res.GetHits())

	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	checkWireStock(t, "RunSavedSearch", run.GetHits())
}
