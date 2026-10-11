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

func TestSearchListings_VisibilityPolicy(t *testing.T) {
	t.Run("a seller's own draft view reaches the index; the request map is untouched", func(t *testing.T) {
		mi := &mockIndex{}
		h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
		ctx := principalCtx("A", "user", "search:read")
		req := &searchv1.SearchListingsRequest{Filters: map[string]string{"seller_id": "A", "status": "draft"}}
		if _, err := h.SearchListings(ctx, req); err != nil {
			t.Fatal(err)
		}
		if mi.lastFilters["status"] != "draft" || mi.lastFilters["seller_id"] != "A" {
			t.Fatalf("index got %v", mi.lastFilters)
		}
		if len(req.Filters) != 2 {
			t.Fatalf("request filters mutated: %v", req.Filters)
		}
	})

	rejects := []struct {
		name    string
		ctx     func() (string, string, []string)
		filters map[string]string
		want    codes.Code
	}{
		{"non-owner", func() (string, string, []string) { return "B", "user", []string{"search:read"} }, map[string]string{"seller_id": "A", "status": "draft"}, codes.PermissionDenied},
		{"anonymous draft", func() (string, string, []string) { return "anonymous", "anonymous", []string{"search:read"} }, map[string]string{"status": "draft"}, codes.Unauthenticated},
		{"draft without seller_id", func() (string, string, []string) { return "A", "user", []string{"search:read"} }, map[string]string{"status": "draft"}, codes.PermissionDenied},
		{"deleted status", func() (string, string, []string) { return "A", "user", []string{"search:read"} }, map[string]string{"status": "deleted"}, codes.InvalidArgument},
	}
	for _, tc := range rejects {
		t.Run("rejects "+tc.name+" without touching the index", func(t *testing.T) {
			mi := &mockIndex{}
			h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
			id, typ, scopes := tc.ctx()
			_, err := h.SearchListings(principalCtx(id, typ, scopes...), &searchv1.SearchListingsRequest{Filters: tc.filters})
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.want)
			}
			if mi.searchCalls != 0 {
				t.Fatal("index was searched")
			}
		})
	}
}

// specs/saved-searches: "A saved draft filter is rejected for a non-owner" plus
// the run-time re-check with the runner's identity.
func TestSavedSearch_VisibilityAtSaveAndRun(t *testing.T) {
	rw := []string{"search:read", "search:write"}

	t.Run("a non-owner cannot save a draft filter and nothing is stored", func(t *testing.T) {
		spy := &spyRepo{inner: repository.NewInMemorySavedSearchRepository()}
		h := handler.NewSearchHandler(&mockIndex{}, spy)
		_, err := h.SaveSearch(principalCtx("B", "user", rw...),
			&searchv1.SaveSearchRequest{Query: "x", FiltersJson: `{"status":"draft"}`})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("code = %v (%v)", status.Code(err), err)
		}
		if spy.calls != 0 {
			t.Fatal("a rejected save reached the repository")
		}
	})

	t.Run("an unknown status is rejected at save time", func(t *testing.T) {
		h := handler.NewSearchHandler(&mockIndex{}, repository.NewInMemorySavedSearchRepository())
		_, err := h.SaveSearch(principalCtx("B", "user", rw...),
			&searchv1.SaveSearchRequest{Query: "x", FiltersJson: `{"status":"active"}`})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code = %v (%v)", status.Code(err), err)
		}
	})

	t.Run("an owner may save and run their draft view", func(t *testing.T) {
		mi := &mockIndex{}
		h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
		ctx := principalCtx("A", "user", rw...)
		saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{FiltersJson: `{"seller_id":"A","status":"draft"}`})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()}); err != nil {
			t.Fatal(err)
		}
		if mi.lastFilters["status"] != "draft" {
			t.Fatalf("run lost the owner draft view: %v", mi.lastFilters)
		}
	})

	t.Run("run re-checks visibility with the runner identity (a stored draft filter of another seller)", func(t *testing.T) {
		repo := repository.NewInMemorySavedSearchRepository()
		mi := &mockIndex{}
		h := handler.NewSearchHandler(mi, repo)
		// A row written before the policy existed (or by a bug) for user B naming A's drafts.
		row, _ := repo.Create(context.Background(), repository.SavedSearch{UserID: "B", Query: "x", FiltersJSON: `{"seller_id":"A","status":"draft"}`})
		_, err := h.RunSavedSearch(principalCtx("B", "user", rw...), &searchv1.RunSavedSearchRequest{Id: row.ID})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("code = %v (%v)", status.Code(err), err)
		}
		if mi.searchCalls != 0 {
			t.Fatal("index searched despite the denied draft filter")
		}
	})

	t.Run("running a query with no status filter leaves the published default to the index", func(t *testing.T) {
		mi := &mockIndex{}
		h := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
		ctx := principalCtx("A", "user", rw...)
		saved, _ := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "phone"})
		if _, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()}); err != nil {
			t.Fatal(err)
		}
		if _, has := mi.lastFilters["status"]; has {
			t.Fatalf("the handler must not inject a status; the index owns the default: %v", mi.lastFilters)
		}
	})
}
