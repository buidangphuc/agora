package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/repository"
)

// D4: filters["in_stock"] reaches the index only as "true"; any other value is
// INVALID_ARGUMENT on SearchListings, SaveSearch and RunSavedSearch.
func TestInStockFilter_Validation(t *testing.T) {
	rw := []string{"search:read", "search:write"}

	t.Run("SearchListings", func(t *testing.T) {
		for v, want := range map[string]codes.Code{"true": codes.OK, "maybe": codes.InvalidArgument, "yes": codes.InvalidArgument} {
			mi := &mockIndex{}
			h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
			_, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"),
				&searchv1.SearchListingsRequest{Query: "x", Filters: map[string]string{"in_stock": v}})
			if status.Code(err) != want {
				t.Fatalf("in_stock=%q: code %v (%v), want %v", v, status.Code(err), err, want)
			}
			if want == codes.OK && mi.lastFilters["in_stock"] != "true" {
				t.Errorf("in_stock not forwarded: %v", mi.lastFilters)
			}
			if want != codes.OK && mi.searchCalls != 0 {
				t.Errorf("in_stock=%q reached the index", v)
			}
		}
	})

	t.Run("SaveSearch", func(t *testing.T) {
		spy := &spyRepo{inner: repository.NewInMemorySavedSearchRepository()}
		h := handler.NewSearchHandler(&mockIndex{}, spy)
		ctx := principalCtx("B", "user", rw...)
		if _, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x", FiltersJson: `{"in_stock":"yes"}`}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code %v (%v)", status.Code(err), err)
		}
		if spy.calls != 0 {
			t.Fatal("an invalid in_stock save reached the repository")
		}
		if _, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x", FiltersJson: `{"in_stock":"true"}`}); err != nil {
			t.Fatalf("valid save: %v", err)
		}
	})

	t.Run("RunSavedSearch", func(t *testing.T) {
		repo := repository.NewInMemorySavedSearchRepository()
		mi := &mockIndex{}
		h := handler.NewSearchHandler(mi, repo)
		ctx := principalCtx("B", "user", rw...)
		good, _ := repo.Create(context.Background(), repository.SavedSearch{UserID: "B", Query: "x", FiltersJSON: `{"in_stock":"true"}`})
		if _, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: good.ID}); err != nil {
			t.Fatalf("run valid: %v", err)
		}
		if mi.lastFilters["in_stock"] != "true" {
			t.Errorf("run lost in_stock: %v", mi.lastFilters)
		}
		// A row stored before validation existed.
		bad, _ := repo.Create(context.Background(), repository.SavedSearch{UserID: "B", Query: "x", FiltersJSON: `{"in_stock":"maybe"}`})
		calls := mi.searchCalls
		if _, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: bad.ID}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("run invalid: code %v (%v)", status.Code(err), err)
		}
		if mi.searchCalls != calls {
			t.Error("invalid stored in_stock reached the index")
		}
	})
}
