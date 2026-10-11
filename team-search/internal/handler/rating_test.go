package handler_test

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/repository"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

// D9: min_rating != 0 is INVALID_ARGUMENT in every search mode, before any
// retrieval runs; min_rating 0 behaves as no rating.
func TestSearchListings_RejectsMinRatingInEveryMode(t *testing.T) {
	ctx := principalCtx("anonymous", "anonymous", "search:read")
	modes := []searchv1.SearchMode{
		searchv1.SearchMode_SEARCH_MODE_UNSPECIFIED, searchv1.SearchMode_SEARCH_MODE_LEXICAL,
		searchv1.SearchMode_SEARCH_MODE_SEMANTIC, searchv1.SearchMode_SEARCH_MODE_HYBRID,
	}
	for _, withEngine := range []bool{false, true} {
		for _, mode := range modes {
			for _, rating := range []int32{4, 1, -1} {
				mi := &mockIndex{}
				h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
				if withEngine {
					eng := retrieval.NewEngine(mi, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
					h = handler.NewSearchHandlerWithEngine(mi, eng, repository.NewInMemorySavedSearchRepository())
				}
				_, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", MinRating: rating, SearchMode: mode})
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("engine=%v mode=%v rating=%d: code %v (%v)", withEngine, mode, rating, status.Code(err), err)
				}
				if mi.searchCalls != 0 {
					t.Fatalf("engine=%v mode=%v: the index was called", withEngine, mode)
				}
			}
			mi := &mockIndex{}
			h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
			if _, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", SearchMode: mode}); err != nil {
				t.Fatalf("min_rating 0 must be accepted: %v", err)
			}
			if mi.lastMinRating != 0 {
				t.Errorf("min_rating 0 must reach the index as 0, got %d", mi.lastMinRating)
			}
		}
	}
}

// D9: the ratings facet is an empty, non-nil list on SearchListings and
// RunSavedSearch, even when the index reports buckets.
func TestRatingsFacetIsEmpty(t *testing.T) {
	h := handler.NewSearchHandler(&mockIndex{}, repository.NewInMemorySavedSearchRepository())
	ctx := principalCtx("B", "user", "search:read", "search:write")
	res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.GetFacets().GetRatings(); r == nil || len(r) != 0 {
		t.Errorf("SearchListings ratings = %v", r)
	}
	if len(res.GetFacets().GetCategories()) == 0 {
		t.Error("categories must still be filled")
	}
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if r := run.GetFacets().GetRatings(); r == nil || len(r) != 0 {
		t.Errorf("RunSavedSearch ratings = %v", r)
	}
}
