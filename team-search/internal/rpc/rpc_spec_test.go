package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/store"
)

const leakText = "dial tcp 10.9.8.7:5432: password authentication failed for user admin"

type failingIndex struct {
	*mockIndex
	err error
}

func (f failingIndex) Search(context.Context, string, map[string]string, string, int64, int64, int32, searchv1.SortBy, int, int) (index.SearchResult, error) {
	return index.SearchResult{}, f.err
}

func (f failingIndex) Suggest(context.Context, string, int) ([]string, error) { return nil, f.err }

func requireGeneric(t *testing.T, what string, err error) {
	t.Helper()
	st := status.Convert(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("%s: got %v %q, want Internal \"internal error\"", what, st.Code(), st.Message())
	}
	if strings.Contains(st.Message(), "10.9.8.7") || strings.Contains(st.Message(), "password") {
		t.Fatalf("%s: message leaks backend text: %q", what, st.Message())
	}
}

// Scenario: Saved searches list newest first with a total.
func TestSavedSearchListNewestFirstWithCursor(t *testing.T) {
	h := newSavedHandler(t)
	ctx := userCtx("buyer_1", "search:read", "search:write")
	for _, q := range []string{"a", "b", "c"} {
		if _, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: q}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range list.GetSavedSearches() {
		got = append(got, s.GetQuery())
	}
	if strings.Join(got, ",") != "c,b,a" || list.GetPage().GetTotal() != 3 || list.GetPage().GetNextCursor() != "" {
		t.Fatalf("queries=%v total=%d next=%q, want c,b,a total 3 and no next cursor", got, list.GetPage().GetTotal(), list.GetPage().GetNextCursor())
	}

	first, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{Page: pageReq("", 2)})
	if err != nil || len(first.GetSavedSearches()) != 2 || first.GetPage().GetNextCursor() != "2" || first.GetPage().GetTotal() != 3 {
		t.Fatalf("first page = %+v err=%v, want 2 rows, cursor 2, total 3", first, err)
	}
	second, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{Page: pageReq("2", 2)})
	if err != nil || len(second.GetSavedSearches()) != 1 || second.GetSavedSearches()[0].GetQuery() != "a" || second.GetPage().GetNextCursor() != "" {
		t.Fatalf("second page = %+v err=%v, want the oldest row and no cursor", second, err)
	}
}

// Scenario: A page size above the maximum is clamped.
func TestSavedSearchListClampsPageSize(t *testing.T) {
	h := newSavedHandler(t)
	ctx := userCtx("buyer_1", "search:read", "search:write")
	for i := range 60 {
		if _, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "q" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{Page: pageReq("", 500)})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetSavedSearches()) != 50 || list.GetPage().GetNextCursor() != "50" || list.GetPage().GetTotal() != 60 {
		t.Fatalf("rows=%d cursor=%q total=%d, want 50, \"50\", 60", len(list.GetSavedSearches()), list.GetPage().GetNextCursor(), list.GetPage().GetTotal())
	}
	rest, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{Page: pageReq("50", 500)})
	if err != nil || len(rest.GetSavedSearches()) != 10 || rest.GetPage().GetNextCursor() != "" {
		t.Fatalf("rest = %d rows next=%q err=%v, want 10 rows and no cursor", len(rest.GetSavedSearches()), rest.GetPage().GetNextCursor(), err)
	}
}

// Scenario: An empty save is rejected.
func TestSaveSearchRejectsEmpty(t *testing.T) {
	spy := &spyRepo{inner: newFakeSaved()}
	h := newHandler(t, &mockIndex{}, spy)
	_, err := h.SaveSearch(userCtx("buyer_1", "search:read", "search:write"), &searchv1.SaveSearchRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v (%v), want InvalidArgument", status.Code(err), err)
	}
	if spy.calls != 0 {
		t.Errorf("an empty save reached the store %d times", spy.calls)
	}
}

// Scenario: A store failure returns a generic internal error.
func TestStoreFailureReturnsGenericInternalError(t *testing.T) {
	h := newHandler(t, &mockIndex{}, failingSaved{err: fmt.Errorf("query saved_searches: %s", leakText)})
	ctx := userCtx("buyer_1", "search:read", "search:write")
	for _, c := range savedSearchRPCs {
		t.Run(c.name, func(t *testing.T) {
			requireGeneric(t, c.name, c.call(h, ctx))
		})
	}
}

// Scenario: Another user's saved search is not found (error table, wrapped sentinel).
func TestStoreNotFoundMapsToNotFound(t *testing.T) {
	h := newHandler(t, &mockIndex{}, failingSaved{err: fmt.Errorf("get row: %w", store.ErrSavedSearchNotFound)})
	ctx := userCtx("buyer_1", "search:read", "search:write")
	for _, c := range savedSearchRPCs {
		if c.name == "SaveSearch" || c.name == "ListSavedSearches" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			st := status.Convert(c.call(h, ctx))
			if st.Code() != codes.NotFound || st.Message() != "saved search not found" {
				t.Fatalf("got %v %q, want NotFound \"saved search not found\"", st.Code(), st.Message())
			}
		})
	}
}

// Scenario: A search backend failure returns a generic internal error.
func TestSearchBackendFailureReturnsGenericInternalError(t *testing.T) {
	idx := failingIndex{mockIndex: &mockIndex{}, err: errors.New(leakText)}
	saved := newFakeSaved()
	h := newHandler(t, idx, saved)
	ctx := userCtx("buyer_1", "search:read", "search:write")

	_, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	requireGeneric(t, "SearchListings", err)
	_, err = h.Suggest(ctx, &searchv1.SuggestRequest{Query: "x"})
	requireGeneric(t, "Suggest", err)

	row, err := saved.Create(t.Context(), store.SavedSearch{UserID: "buyer_1", Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: row.ID})
	requireGeneric(t, "RunSavedSearch", err)
}

// Scenario: A cancelled request returns Canceled.
func TestContextCanceledMapsToCanceled(t *testing.T) {
	ctx := userCtx("buyer_1", "search:read", "search:write")
	wrapped := fmt.Errorf("opensearch round trip: %w", context.Canceled)

	h := newHandler(t, failingIndex{mockIndex: &mockIndex{}, err: wrapped}, newFakeSaved())
	if _, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"}); status.Code(err) != codes.Canceled {
		t.Errorf("SearchListings: code = %v (%v), want Canceled", status.Code(err), err)
	}
	if _, err := h.Suggest(ctx, &searchv1.SuggestRequest{Query: "x"}); status.Code(err) != codes.Canceled {
		t.Errorf("Suggest: code = %v (%v), want Canceled", status.Code(err), err)
	}

	hs := newHandler(t, &mockIndex{}, failingSaved{err: wrapped})
	if _, err := hs.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{}); status.Code(err) != codes.Canceled {
		t.Errorf("ListSavedSearches: code = %v (%v), want Canceled", status.Code(err), err)
	}

	hd := newHandler(t, failingIndex{mockIndex: &mockIndex{}, err: fmt.Errorf("slow: %w", context.DeadlineExceeded)}, newFakeSaved())
	if _, err := hd.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"}); status.Code(err) != codes.DeadlineExceeded {
		t.Errorf("deadline: code = %v (%v), want DeadlineExceeded", status.Code(err), err)
	}
}

// Scenario: A draft filter by a stranger keeps its visibility error (status
// errors pass through the table unchanged, never turned into Internal).
func TestVisibilityErrorPassesThroughStatusTable(t *testing.T) {
	cases := []struct {
		name    string
		ctx     context.Context
		filters map[string]string
		want    codes.Code
	}{
		{"stranger draft", userCtx("B", "search:read"), map[string]string{"seller_id": "A", "status": "draft"}, codes.PermissionDenied},
		{"anonymous draft", principalCtx("anonymous", "anonymous", "search:read"), map[string]string{"status": "draft"}, codes.Unauthenticated},
		{"deleted status", userCtx("A", "search:read"), map[string]string{"status": "deleted"}, codes.InvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mi := &mockIndex{}
			h := newHandler(t, mi, newFakeSaved())
			_, err := h.SearchListings(c.ctx, &searchv1.SearchListingsRequest{Filters: c.filters})
			st := status.Convert(err)
			if st.Code() != c.want {
				t.Fatalf("code = %v (%q), want %v", st.Code(), st.Message(), c.want)
			}
			if st.Message() == "internal error" || st.Message() == "" {
				t.Errorf("visibility message was replaced: %q", st.Message())
			}
			if mi.searchCalls != 0 {
				t.Error("index searched")
			}
		})
	}
}

// Scenario: An anonymous principal holding search:read can search and get suggestions.
func TestSearchAnonymousWithScopeSucceeds(t *testing.T) {
	h := newHandler(t, &mockIndex{}, newFakeSaved())
	res, err := h.SearchListings(principalCtx("anonymous", "anonymous", "listing.read", "search:read"), &searchv1.SearchListingsRequest{Query: "iPhone"})
	if err != nil {
		t.Fatalf("anonymous search: %v", err)
	}
	if len(res.GetHits()) != 2 {
		t.Errorf("hits = %d, want 2", len(res.GetHits()))
	}
}

// Scenario: An anonymous principal holding search:read can search and get suggestions.
func TestSuggestAnonymousWithScopeSucceeds(t *testing.T) {
	h := newHandler(t, &mockIndex{}, newFakeSaved())
	res, err := h.Suggest(principalCtx("anonymous", "anonymous", "search:read"), &searchv1.SuggestRequest{Query: "iPh"})
	if err != nil {
		t.Fatalf("anonymous suggest: %v", err)
	}
	if len(res.GetSuggestions()) != 3 {
		t.Errorf("suggestions = %d, want 3", len(res.GetSuggestions()))
	}
}

// Scenario: Anonymous browsing is not blocked by the SDK gate (the gate still
// demands search:read, and a call with no principal at all is Unauthenticated).
func TestSearchAndSuggestGateOnScopeNotIdentity(t *testing.T) {
	calls := map[string]func(ctx context.Context) error{
		"SearchListings": func(ctx context.Context) error {
			_, err := newHandler(t, &mockIndex{}, newFakeSaved()).SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
			return err
		},
		"Suggest": func(ctx context.Context) error {
			_, err := newHandler(t, &mockIndex{}, newFakeSaved()).Suggest(ctx, &searchv1.SuggestRequest{Query: "x"})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(principalCtx("anonymous", "anonymous", "listing.read")); status.Code(err) != codes.PermissionDenied {
				t.Errorf("anonymous without search:read: %v, want PermissionDenied", err)
			}
			if err := call(principalCtx("", "")); status.Code(err) != codes.Unauthenticated {
				t.Errorf("no principal: %v, want Unauthenticated", err)
			}
			if err := call(userCtx("buyer_1", "search:read")); err != nil {
				t.Errorf("signed-in user with search:read: %v", err)
			}
		})
	}
}

// Scenario: An anonymous principal asking for status=draft keeps its visibility error.
func TestSearchAnonymousDraftFilterKeepsVisibilityError(t *testing.T) {
	mi := &mockIndex{}
	h := newHandler(t, mi, newFakeSaved())
	_, err := h.SearchListings(principalCtx("anonymous", "anonymous", "search:read"),
		&searchv1.SearchListingsRequest{Filters: map[string]string{"status": "draft"}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v (%v), want Unauthenticated", status.Code(err), err)
	}
	if mi.searchCalls != 0 {
		t.Error("index searched for a rejected draft filter")
	}
}
