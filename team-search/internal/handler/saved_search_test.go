package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/repository"
)

// userCtx builds a gateway-style principal context with the given id + scopes.
func userCtx(id string, scopes ...string) context.Context {
	return contextWithPrincipal(context.Background(), &commonv1.Principal{
		Id:     id,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: scopes,
	})
}

func newSavedHandler() *handler.SearchHandler {
	return handler.NewSearchHandler(&mockIndex{}, repository.NewInMemorySavedSearchRepository())
}

// TestSaveThenList covers the happy path: a save is persisted and shows up in
// the owner's list with the same query/filters.
func TestSaveThenList(t *testing.T) {
	h := newSavedHandler()
	ctx := userCtx("buyer_1", "search:read", "search:write")

	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{
		Query:       "iPhone",
		FiltersJson: `{"status":"active"}`,
	})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	if saved.GetSavedSearch().GetId() == "" {
		t.Fatal("expected generated id")
	}
	if saved.GetSavedSearch().GetQuery() != "iPhone" {
		t.Errorf("query mismatch: %q", saved.GetSavedSearch().GetQuery())
	}

	list, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
	if err != nil {
		t.Fatalf("ListSavedSearches: %v", err)
	}
	if list.GetPage().GetTotal() != 1 || len(list.GetSavedSearches()) != 1 {
		t.Fatalf("expected 1 saved search, got total=%d len=%d", list.GetPage().GetTotal(), len(list.GetSavedSearches()))
	}
	if list.GetSavedSearches()[0].GetFiltersJson() != `{"status":"active"}` {
		t.Errorf("filters mismatch: %q", list.GetSavedSearches()[0].GetFiltersJson())
	}
}

// TestDeleteSavedSearch verifies delete removes the row and a second delete 404s.
func TestDeleteSavedSearch(t *testing.T) {
	h := newSavedHandler()
	ctx := userCtx("buyer_1", "search:read", "search:write")

	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "sofa"})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	id := saved.GetSavedSearch().GetId()

	if _, err := h.DeleteSavedSearch(ctx, &searchv1.DeleteSavedSearchRequest{Id: id}); err != nil {
		t.Fatalf("DeleteSavedSearch: %v", err)
	}
	list, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
	if err != nil {
		t.Fatalf("ListSavedSearches: %v", err)
	}
	if list.GetPage().GetTotal() != 0 {
		t.Errorf("expected 0 after delete, got %d", list.GetPage().GetTotal())
	}
	// Deleting again is NotFound.
	_, err = h.DeleteSavedSearch(ctx, &searchv1.DeleteSavedSearchRequest{Id: id})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound on second delete, got %v", err)
	}
}

// TestRunSavedSearch runs a saved search back through the index path and expects
// the SearchListings-shaped results (the mock index returns two hits + facets).
func TestRunSavedSearch(t *testing.T) {
	h := newSavedHandler()
	ctx := userCtx("buyer_1", "search:read", "search:write")

	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{
		Query:       "iPhone",
		FiltersJson: `{"category_id":"cat_phones"}`,
	})
	if err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}

	res, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: saved.GetSavedSearch().GetId()})
	if err != nil {
		t.Fatalf("RunSavedSearch: %v", err)
	}
	if len(res.GetHits()) != 2 {
		t.Errorf("expected 2 hits, got %d", len(res.GetHits()))
	}
	if res.GetPage().GetTotal() != 2 {
		t.Errorf("expected total 2, got %d", res.GetPage().GetTotal())
	}
	if res.GetFacets() == nil || len(res.GetFacets().GetCategories()) != 1 {
		t.Errorf("expected facets echoed from index path, got %+v", res.GetFacets())
	}
}

// TestCrossUserIsolation proves one user can neither list, run, nor delete
// another user's saved search.
func TestCrossUserIsolation(t *testing.T) {
	h := newSavedHandler()
	alice := userCtx("alice", "search:read", "search:write")
	bob := userCtx("bob", "search:read", "search:write")

	saved, err := h.SaveSearch(alice, &searchv1.SaveSearchRequest{Query: "villa"})
	if err != nil {
		t.Fatalf("SaveSearch(alice): %v", err)
	}
	id := saved.GetSavedSearch().GetId()

	// Bob's list is empty.
	bobList, err := h.ListSavedSearches(bob, &searchv1.ListSavedSearchesRequest{})
	if err != nil {
		t.Fatalf("ListSavedSearches(bob): %v", err)
	}
	if bobList.GetPage().GetTotal() != 0 {
		t.Errorf("bob should see 0 saved searches, got %d", bobList.GetPage().GetTotal())
	}

	// Bob cannot run alice's saved search.
	if _, err := h.RunSavedSearch(bob, &searchv1.RunSavedSearchRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Errorf("bob run of alice's search: expected NotFound, got %v", err)
	}
	// Bob cannot delete alice's saved search.
	if _, err := h.DeleteSavedSearch(bob, &searchv1.DeleteSavedSearchRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Errorf("bob delete of alice's search: expected NotFound, got %v", err)
	}
	// Alice's row survives.
	aliceList, err := h.ListSavedSearches(alice, &searchv1.ListSavedSearchesRequest{})
	if err != nil {
		t.Fatalf("ListSavedSearches(alice): %v", err)
	}
	if aliceList.GetPage().GetTotal() != 1 {
		t.Errorf("alice should still have 1 saved search, got %d", aliceList.GetPage().GetTotal())
	}
}

// TestSavedSearchAuth covers anonymous/insufficient-scope and empty-input edges.
func TestSavedSearchAuth(t *testing.T) {
	h := newSavedHandler()

	// No principal at all (anonymous, bypassing the gateway) → Unauthenticated.
	if _, err := h.ListSavedSearches(context.Background(), &searchv1.ListSavedSearchesRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("anonymous list: expected Unauthenticated, got %v", err)
	}

	// Read scope only cannot write.
	readOnly := userCtx("buyer_1", "search:read")
	if _, err := h.SaveSearch(readOnly, &searchv1.SaveSearchRequest{Query: "x"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("read-only save: expected PermissionDenied, got %v", err)
	}

	// Empty save (no query, no filters) is rejected without panicking.
	rw := userCtx("buyer_1", "search:read", "search:write")
	if _, err := h.SaveSearch(rw, &searchv1.SaveSearchRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty save: expected InvalidArgument, got %v", err)
	}
}
