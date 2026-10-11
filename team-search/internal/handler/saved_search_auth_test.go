package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/repository"
)

type rpcCall struct {
	name string
	call func(h *handler.SearchHandler, ctx context.Context) error
}

var savedSearchRPCs = []rpcCall{
	{"SaveSearch", func(h *handler.SearchHandler, ctx context.Context) error {
		_, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "x"})
		return err
	}},
	{"ListSavedSearches", func(h *handler.SearchHandler, ctx context.Context) error {
		_, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
		return err
	}},
	{"DeleteSavedSearch", func(h *handler.SearchHandler, ctx context.Context) error {
		_, err := h.DeleteSavedSearch(ctx, &searchv1.DeleteSavedSearchRequest{Id: "some-id"})
		return err
	}},
	{"RunSavedSearch", func(h *handler.SearchHandler, ctx context.Context) error {
		_, err := h.RunSavedSearch(ctx, &searchv1.RunSavedSearchRequest{Id: "some-id"})
		return err
	}},
}

// Every RPC x every caller shape: the gRPC code is the contract, and a denied
// call must touch neither the repository nor the index.
func TestSavedSearchCallerIdentity_Matrix(t *testing.T) {
	const both = "search:read,search:write"
	callers := []struct {
		name string
		ctx  context.Context
		want codes.Code // for a scope-complete happy path, RPC-specific NotFound handled below
	}{
		{"no principal", principalCtx("", ""), codes.Unauthenticated},
		{"anonymous with search:read", principalCtx("anonymous", "anonymous", "listing.read", "search:read"), codes.Unauthenticated},
		{"anonymous with search:write", principalCtx("anonymous", "anonymous", "search:write"), codes.Unauthenticated},
		{"anonymous with both", principalCtx("anonymous", "anonymous", strings.Split(both, ",")...), codes.Unauthenticated},
		{"unspecified type with both", principalCtx("u1", "", strings.Split(both, ",")...), codes.Unauthenticated},
		{"user type but reserved anonymous id", principalCtx("anonymous", "user", strings.Split(both, ",")...), codes.Unauthenticated},
		{"service with both scopes", principalCtx("svc", "service", strings.Split(both, ",")...), codes.PermissionDenied},
	}
	for _, rpc := range savedSearchRPCs {
		for _, c := range callers {
			t.Run(rpc.name+"/"+c.name, func(t *testing.T) {
				mi := &mockIndex{}
				spy := &spyRepo{inner: repository.NewInMemorySavedSearchRepository()}
				h := handler.NewSearchHandler(mi, spy)
				err := rpc.call(h, c.ctx)
				if status.Code(err) != c.want {
					t.Fatalf("code = %v (%v), want %v", status.Code(err), err, c.want)
				}
				if spy.calls != 0 {
					t.Errorf("repository touched %d times by a denied call", spy.calls)
				}
				if mi.searchCalls != 0 {
					t.Errorf("index searched %d times by a denied call", mi.searchCalls)
				}
			})
		}
	}
}

// A signed-in user missing the RPC's scope gets PERMISSION_DENIED (identity passes,
// the scope gate still applies after it); with the scope the call reaches storage.
func TestSavedSearchScopes_AfterIdentity(t *testing.T) {
	needs := map[string]string{
		"SaveSearch": "search:write", "DeleteSavedSearch": "search:write",
		"ListSavedSearches": "search:read", "RunSavedSearch": "search:read",
	}
	for _, rpc := range savedSearchRPCs {
		t.Run(rpc.name, func(t *testing.T) {
			spy := &spyRepo{inner: repository.NewInMemorySavedSearchRepository()}
			h := handler.NewSearchHandler(&mockIndex{}, spy)
			other := "search:read"
			if needs[rpc.name] == "search:read" {
				other = "search:write"
			}
			err := rpc.call(h, principalCtx("buyer_1", "user", other))
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("missing scope: code = %v (%v), want PermissionDenied", status.Code(err), err)
			}
			if spy.calls != 0 {
				t.Errorf("repository touched by a scope-denied call")
			}
			err = rpc.call(h, principalCtx("buyer_1", "user", needs[rpc.name]))
			if c := status.Code(err); c == codes.Unauthenticated || c == codes.PermissionDenied {
				t.Fatalf("with the right scope the call must pass the gates, got %v", err)
			}
			if spy.calls == 0 {
				t.Errorf("an authorised call should reach the repository")
			}
		})
	}
}

// A signed-in user with both scopes saves and then lists (spec happy path).
func TestSavedSearch_UserWithScopesSucceeds(t *testing.T) {
	h := handler.NewSearchHandler(&mockIndex{}, repository.NewInMemorySavedSearchRepository())
	ctx := principalCtx("buyer_1", "user", "search:read", "search:write")
	saved, err := h.SaveSearch(ctx, &searchv1.SaveSearchRequest{Query: "iPhone"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := h.ListSavedSearches(ctx, &searchv1.ListSavedSearchesRequest{})
	if err != nil || list.GetPage().GetTotal() != 1 || list.GetSavedSearches()[0].GetId() != saved.GetSavedSearch().GetId() {
		t.Fatalf("list = %+v err=%v", list, err)
	}
}
