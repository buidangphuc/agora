package rpc

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/store"
)

func requireSavedUser(ctx context.Context, scope string) (auth.Principal, error) {
	if _, err := auth.RequireUser(ctx); err != nil {
		return auth.Principal{}, err
	}
	return auth.RequireScopes(ctx, scope)
}

func (h *Handler) SaveSearch(ctx context.Context, req *searchv1.SaveSearchRequest) (*searchv1.SaveSearchResponse, error) {
	p, err := requireSavedUser(ctx, scopeSearchWrite)
	if err != nil {
		return nil, err
	}
	filtersJSON := req.GetFiltersJson()
	if req.GetQuery() == "" && !hasFilters(filtersJSON) {
		return nil, status.Error(codes.InvalidArgument, "query or filters_json required")
	}
	parsed, err := parseFilters(filtersJSON)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "filters_json must be a JSON object of string values")
	}
	if _, err := effectiveFilters(p, parsed); err != nil {
		return nil, err
	}
	saved, err := h.saved.Create(ctx, store.SavedSearch{UserID: p.ID, Query: req.GetQuery(), FiltersJSON: filtersJSON})
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &searchv1.SaveSearchResponse{SavedSearch: toWireSavedSearch(saved)}, nil
}

func (h *Handler) ListSavedSearches(ctx context.Context, req *searchv1.ListSavedSearchesRequest) (*searchv1.ListSavedSearchesResponse, error) {
	p, err := requireSavedUser(ctx, scopeSearchRead)
	if err != nil {
		return nil, err
	}
	from, size := decodePage(req.GetPage())
	items, total, err := h.saved.List(ctx, p.ID, size, from)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	wire := make([]*searchv1.SavedSearch, 0, len(items))
	for _, s := range items {
		wire = append(wire, toWireSavedSearch(s))
	}
	return &searchv1.ListSavedSearchesResponse{
		SavedSearches: wire,
		Page:          &commonv1.PageResponse{NextCursor: nextCursor(from, size, total), Total: total},
	}, nil
}

func (h *Handler) DeleteSavedSearch(ctx context.Context, req *searchv1.DeleteSavedSearchRequest) (*searchv1.DeleteSavedSearchResponse, error) {
	p, err := requireSavedUser(ctx, scopeSearchWrite)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	if err := h.saved.Delete(ctx, req.GetId(), p.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &searchv1.DeleteSavedSearchResponse{}, nil
}

func (h *Handler) RunSavedSearch(ctx context.Context, req *searchv1.RunSavedSearchRequest) (*searchv1.RunSavedSearchResponse, error) {
	p, err := requireSavedUser(ctx, scopeSearchRead)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	saved, err := h.saved.Get(ctx, req.GetId(), p.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	filters, err := parseFilters(saved.FiltersJSON)
	if err != nil {
		filters = nil
	}
	filters, err = effectiveFilters(p, filters)
	if err != nil {
		return nil, err
	}
	res, err := h.idx.Search(ctx, saved.Query, filters, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, defaultPageSize)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &searchv1.RunSavedSearchResponse{
		Hits:   toWireHits(res.Hits),
		Page:   &commonv1.PageResponse{NextCursor: nextCursor(0, defaultPageSize, res.Total), Total: res.Total},
		Facets: toFacets(res.Facets),
	}, nil
}

func hasFilters(filtersJSON string) bool {
	m, err := parseFilters(filtersJSON)
	return err == nil && len(m) > 0
}

func parseFilters(filtersJSON string) (map[string]string, error) {
	if filtersJSON == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(filtersJSON), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func toWireSavedSearch(s store.SavedSearch) *searchv1.SavedSearch {
	out := &searchv1.SavedSearch{Id: s.ID, Query: s.Query, FiltersJson: s.FiltersJSON}
	if !s.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(s.CreatedAt)
	}
	return out
}
