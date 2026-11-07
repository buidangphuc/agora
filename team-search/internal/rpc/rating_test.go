package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/store"
)

var allModes = []searchv1.SearchMode{
	searchv1.SearchMode_SEARCH_MODE_UNSPECIFIED, searchv1.SearchMode_SEARCH_MODE_LEXICAL,
	searchv1.SearchMode_SEARCH_MODE_SEMANTIC, searchv1.SearchMode_SEARCH_MODE_HYBRID,
}

func TestSearchListings_BR17_RejectsOutOfRangeMinRatingInEveryMode(t *testing.T) {
	ctx := principalCtx("anonymous", "anonymous", "search:read")
	for _, withEngine := range []bool{false, true} {
		for _, mode := range allModes {
			for _, rating := range []int32{6, -1, 100, -100} {
				mi := &mockIndex{}
				h := newHandler(t, mi, newFakeSaved())
				if withEngine {
					eng := retrieval.NewEngine(mi, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
					h = newHandlerEngine(t, mi, eng, newFakeSaved())
				}
				_, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", MinRating: rating, SearchMode: mode})
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("engine=%v mode=%v rating=%d: code %v (%v)", withEngine, mode, rating, status.Code(err), err)
				}
				if mi.searchCalls != 0 {
					t.Fatalf("engine=%v mode=%v rating=%d: the index was called", withEngine, mode, rating)
				}
			}
		}
	}
}

func TestSearchListings_BR13_BR16_AcceptsMinRating0To5AndPassesItToTheIndex(t *testing.T) {
	ctx := principalCtx("anonymous", "anonymous", "search:read")
	for _, withEngine := range []bool{false, true} {
		for _, mode := range []searchv1.SearchMode{
			searchv1.SearchMode_SEARCH_MODE_UNSPECIFIED, searchv1.SearchMode_SEARCH_MODE_LEXICAL, searchv1.SearchMode_SEARCH_MODE_HYBRID,
		} {
			for rating := int32(0); rating <= 5; rating++ {
				mi := &mockIndex{}
				h := newHandler(t, mi, newFakeSaved())
				if withEngine {
					eng := retrieval.NewEngine(mi, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
					h = newHandlerEngine(t, mi, eng, newFakeSaved())
				}
				if _, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", MinRating: rating, SearchMode: mode}); err != nil {
					t.Fatalf("engine=%v mode=%v rating=%d: %v", withEngine, mode, rating, err)
				}
				if mi.searchCalls == 0 || mi.lastMinRating != rating {
					t.Errorf("engine=%v mode=%v: index got min_rating %d (calls %d), want %d", withEngine, mode, mi.lastMinRating, mi.searchCalls, rating)
				}
			}
		}
	}
}

func TestSearchListings_BR16_SemanticModeRatingReachesTheVectorLeg(t *testing.T) {
	mi := &vecRatingIndex{}
	eng := retrieval.NewEngine(mi, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
	h := newHandlerEngine(t, mi, eng, newFakeSaved())
	if _, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"),
		&searchv1.SearchListingsRequest{Query: "x", MinRating: 4, SearchMode: searchv1.SearchMode_SEARCH_MODE_SEMANTIC}); err != nil {
		t.Fatal(err)
	}
	if !mi.vecCalled || mi.vecMinRating != 4 {
		t.Errorf("vector leg: called=%v min_rating=%d, want 4", mi.vecCalled, mi.vecMinRating)
	}
}

type vecRatingIndex struct {
	mockIndex
	vecCalled    bool
	vecMinRating int32
}

func (v *vecRatingIndex) SearchVector(_ context.Context, _ []float32, _ map[string]string, _ string, _, _ int64, minRating int32, _ searchv1.SortBy, _, _ int) (index.SearchResult, error) {
	v.vecCalled = true
	v.vecMinRating = minRating
	return index.SearchResult{Hits: []index.Hit{{ListingID: "v"}}, Total: 1}, nil
}

func TestSearchListings_BR9_BR11_BestSellingReachesTheIndexInEveryMode(t *testing.T) {
	ctx := principalCtx("anonymous", "anonymous", "search:read")
	for _, withEngine := range []bool{false, true} {
		for _, mode := range allModes {
			mi := &mockIndex{}
			h := newHandler(t, mi, newFakeSaved())
			if withEngine {
				eng := retrieval.NewEngine(mi, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
				h = newHandlerEngine(t, mi, eng, newFakeSaved())
			}
			if _, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", SortBy: searchv1.SortBy_SORT_BY_BEST_SELLING, SearchMode: mode}); err != nil {
				t.Fatalf("engine=%v mode=%v: %v", withEngine, mode, err)
			}
			if mi.lastSort != searchv1.SortBy_SORT_BY_BEST_SELLING {
				t.Errorf("engine=%v mode=%v: index sort = %v", withEngine, mode, mi.lastSort)
			}
		}
	}
}

func TestSearchListings_BR12_BestSellingDoesNotShowADraftToAStranger(t *testing.T) {
	mi := &mockIndex{}
	h := newHandler(t, mi, newFakeSaved())
	_, err := h.SearchListings(principalCtx("B", "user", "search:read"), &searchv1.SearchListingsRequest{
		SortBy: searchv1.SortBy_SORT_BY_BEST_SELLING, Filters: map[string]string{"seller_id": "A", "status": "draft"},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
	if mi.searchCalls != 0 {
		t.Error("index was searched")
	}
}

func TestRatingsFacet_BR18_PassesIndexBucketsOnBothRPCs(t *testing.T) {
	h := newHandler(t, &mockIndex{}, newFakeSaved())
	ctx := principalCtx("B", "user", "search:read", "search:write")
	res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(rpc string, r []*searchv1.FacetBucket) {
		t.Helper()
		if len(r) != 2 || r[0].GetKey() != "4" || r[0].GetCount() != 2 || r[1].GetKey() != "3" || r[1].GetCount() != 2 {
			t.Errorf("%s ratings = %v", rpc, r)
		}
	}
	check("SearchListings", res.GetFacets().GetRatings())
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	check("RunSavedSearch", run.GetFacets().GetRatings())
}

func TestRatingsFacet_BR18_EmptyIndexFacetIsANonNilList(t *testing.T) {
	h := newHandler(t, &noRatingsIndex{}, newFakeSaved())
	res, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"), &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.GetFacets().GetRatings(); r == nil || len(r) != 0 {
		t.Errorf("ratings = %v, want an empty non-nil list", r)
	}
}

type noRatingsIndex struct{ mockIndex }

func (n *noRatingsIndex) Search(context.Context, string, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	return index.SearchResult{}, nil
}

type ratedIndex struct{ mockIndex }

func (r *ratedIndex) Search(context.Context, string, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	return index.SearchResult{Total: 2, Hits: []index.Hit{
		{ListingID: "rated", SoldCount: 5, RatingAvg: 4.5, ReviewCount: 2},
		{ListingID: "plain"},
	}}, nil
}

func checkWireRating(t *testing.T, rpc string, hits []*searchv1.SearchHit) {
	t.Helper()
	if len(hits) != 2 {
		t.Fatalf("%s: %d hits", rpc, len(hits))
	}
	if h := hits[0]; h.GetSoldCount() != 5 || h.GetRatingAvg() != 4.5 || h.GetReviewCount() != 2 {
		t.Errorf("%s: rated hit = %d/%v/%d, want 5/4.5/2", rpc, h.GetSoldCount(), h.GetRatingAvg(), h.GetReviewCount())
	}
	if h := hits[1]; h.GetSoldCount() != 0 || h.GetRatingAvg() != 0 || h.GetReviewCount() != 0 {
		t.Errorf("%s: untouched hit = %d/%v/%d, want zeros", rpc, h.GetSoldCount(), h.GetRatingAvg(), h.GetReviewCount())
	}
}

func TestSearchHit_BR19_BR20_MapsSoldCountRatingAndReviewCountWithZeros(t *testing.T) {
	h := newHandler(t, &ratedIndex{}, newFakeSaved())
	ctx := principalCtx("B", "user", "search:read", "search:write")
	res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	checkWireRating(t, "SearchListings", res.GetHits())
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	checkWireRating(t, "RunSavedSearch", run.GetHits())
	eng := retrieval.NewEngine(&ratedIndex{}, &retrieval.MockEmbedClient{Dim: 4}, nil, config.Retrieval{EnableHybridSearch: true})
	he := newHandlerEngine(t, &ratedIndex{}, eng, newFakeSaved())
	res, err = he.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL})
	if err != nil {
		t.Fatal(err)
	}
	checkWireRating(t, "SearchListings via engine", res.GetHits())
}

func seedSaved(t *testing.T, user, filtersJSON string) (*mockIndex, *fakeSaved, string, func(string) (*searchv1.RunSavedSearchResponse, error)) {
	t.Helper()
	mi := &mockIndex{}
	repo := newFakeSaved()
	row, err := repo.Create(context.Background(), store.SavedSearch{UserID: user, Query: "x", FiltersJSON: filtersJSON})
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, mi, repo)
	run := func(id string) (*searchv1.RunSavedSearchResponse, error) {
		return h.RunSavedSearch(principalCtx(user, "user", "search:read", "search:write"), &searchv1.RunSavedSearchRequest{Id: id})
	}
	return mi, repo, row.ID, run
}

func TestRunSavedSearch_BR18b_RejectsAnOutOfRangeStoredMinRating(t *testing.T) {
	for _, v := range []string{"7", "6", "-1"} {
		t.Run(v, func(t *testing.T) {
			mi, _, id, run := seedSaved(t, "U", `{"min_rating":"`+v+`"}`)
			_, err := run(id)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v (%v), want InvalidArgument", status.Code(err), err)
			}
			if mi.searchCalls != 0 {
				t.Error("retrieval ran")
			}
		})
	}
}

func TestRunSavedSearch_BR16_AppliesAValidStoredMinRating(t *testing.T) {
	for _, tc := range []struct {
		stored string
		want   int32
	}{{"4", 4}, {"0", 0}, {"5", 5}} {
		mi, _, id, run := seedSaved(t, "U", `{"min_rating":"`+tc.stored+`"}`)
		if _, err := run(id); err != nil {
			t.Fatalf("stored %s: %v", tc.stored, err)
		}
		if mi.searchCalls != 1 || mi.lastMinRating != tc.want {
			t.Errorf("stored %s: index got min_rating %d (calls %d), want %d", tc.stored, mi.lastMinRating, mi.searchCalls, tc.want)
		}
	}
}

func TestRunSavedSearch_BR16_NoStoredMinRatingMeansNoFilter(t *testing.T) {
	mi, _, id, run := seedSaved(t, "U", `{}`)
	if _, err := run(id); err != nil {
		t.Fatal(err)
	}
	if mi.lastMinRating != 0 {
		t.Errorf("min_rating = %d, want 0", mi.lastMinRating)
	}
}
