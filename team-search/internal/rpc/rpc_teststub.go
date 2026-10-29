package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/store"
)

// TEST-STUB: compile-only placeholder for task 1.4; the implementer replaces this file.

type SavedSearches interface {
	Create(ctx context.Context, s store.SavedSearch) (store.SavedSearch, error)
	List(ctx context.Context, userID string, limit, offset int) ([]store.SavedSearch, int64, error)
	Get(ctx context.Context, id, userID string) (store.SavedSearch, error)
	Delete(ctx context.Context, id, userID string) error
}

type Handler struct{}

var errStub = status.Error(codes.Unimplemented, "test stub")

func NewHandler(index.Index, *retrieval.Engine, SavedSearches, *slog.Logger) (*Handler, error) {
	return &Handler{}, nil
}

func (*Handler) SearchListings(context.Context, *searchv1.SearchListingsRequest) (*searchv1.SearchListingsResponse, error) {
	return nil, errStub
}

func (*Handler) Suggest(context.Context, *searchv1.SuggestRequest) (*searchv1.SuggestResponse, error) {
	return nil, errStub
}

func (*Handler) SaveSearch(context.Context, *searchv1.SaveSearchRequest) (*searchv1.SaveSearchResponse, error) {
	return nil, errStub
}

func (*Handler) ListSavedSearches(context.Context, *searchv1.ListSavedSearchesRequest) (*searchv1.ListSavedSearchesResponse, error) {
	return nil, errStub
}

func (*Handler) DeleteSavedSearch(context.Context, *searchv1.DeleteSavedSearchRequest) (*searchv1.DeleteSavedSearchResponse, error) {
	return nil, errStub
}

func (*Handler) RunSavedSearch(context.Context, *searchv1.RunSavedSearchRequest) (*searchv1.RunSavedSearchResponse, error) {
	return nil, errStub
}

func effectiveFilters(auth.Principal, map[string]string) (map[string]string, error) {
	return nil, errStub
}
