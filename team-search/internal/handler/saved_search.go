// Saved-search RPCs for SearchService. These are the one user-owned surface of
// team-search: each saved search belongs to a principal and is stored in the
// service's own Postgres table (repository), separate from the OpenSearch
// read-model. RunSavedSearch loads a saved query/filters and REUSES the existing
// SearchListings index path — it never re-implements search.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"

	"github.com/buidangphuc/team-search/internal/interceptor"
	"github.com/buidangphuc/team-search/internal/repository"
)

// Scopes gating the saved-search surface: reads/runs need search:read (mirrors
// SearchListings), mutations need search:write.
const (
	scopeSearchRead  = "search:read"
	scopeSearchWrite = "search:write"
)

// callerID resolves the authenticated owner of a saved-search call. Identity is
// a precondition, checked BEFORE scopes, so the outcome for an anonymous caller
// does not depend on which scopes the gateway grants it:
//
//  1. a principal must be present, else UNAUTHENTICATED;
//  2. its type must be user: anonymous/unspecified -> UNAUTHENTICATED,
//     service -> PERMISSION_DENIED;
//  3. its id must be non-empty and not the reserved anonymous id, else UNAUTHENTICATED;
//  4. only then the scope gate (search:read for list/run, search:write for mutations).
//
// No saved-search RPC therefore ever runs against a shared or default owner.
func (h *SearchHandler) callerID(ctx context.Context, scope string) (string, error) {
	p, ok := interceptor.PrincipalFromContext(ctx)
	if !ok || p == nil {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	switch p.GetType() {
	case commonv1.PrincipalType_PRINCIPAL_TYPE_USER:
	case commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE:
		return "", status.Error(codes.PermissionDenied, "saved searches are available to signed-in users only")
	default: // anonymous, unspecified
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	if p.GetId() == "" || p.GetId() == anonymousPrincipalID {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	if err := interceptor.RequireScopes(ctx, scope); err != nil {
		return "", err
	}
	return p.GetId(), nil
}

// SaveSearch persists the caller's query + filters for later re-run.
func (h *SearchHandler) SaveSearch(
	ctx context.Context,
	req *searchv1.SaveSearchRequest,
) (*searchv1.SaveSearchResponse, error) {
	userID, err := h.callerID(ctx, scopeSearchWrite)
	if err != nil {
		return nil, err
	}
	if h.saved == nil {
		return nil, status.Error(codes.Unimplemented, "saved searches not configured")
	}
	// Reject a wholly empty save (nothing to re-run) and validate the filters
	// blob up front so RunSavedSearch can always parse it.
	filtersJSON := req.GetFiltersJson()
	if req.GetQuery() == "" && !hasFilters(filtersJSON) {
		return nil, status.Error(codes.InvalidArgument, "query or filters_json required")
	}
	parsed, err := parseFilters(filtersJSON)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "filters_json must be a JSON object of string values")
	}
	// Same visibility rules as SearchListings for this caller, so a filter set that
	// search would reject (for example status=draft for a non-owner) is never stored.
	p, _ := interceptor.PrincipalFromContext(ctx)
	if _, err := effectiveFilters(p, parsed); err != nil {
		return nil, err
	}
	saved, err := h.saved.Create(ctx, repository.SavedSearch{
		UserID:      userID,
		Query:       req.GetQuery(),
		FiltersJSON: filtersJSON,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "save search failed")
	}
	return &searchv1.SaveSearchResponse{SavedSearch: toWireSavedSearch(saved)}, nil
}

// ListSavedSearches returns a page of the caller's saved searches, newest first.
func (h *SearchHandler) ListSavedSearches(
	ctx context.Context,
	req *searchv1.ListSavedSearchesRequest,
) (*searchv1.ListSavedSearchesResponse, error) {
	userID, err := h.callerID(ctx, scopeSearchRead)
	if err != nil {
		return nil, err
	}
	if h.saved == nil {
		return nil, status.Error(codes.Unimplemented, "saved searches not configured")
	}
	from, size := decodePage(req.GetPage())
	items, total, err := h.saved.List(ctx, userID, size, from)
	if err != nil {
		return nil, status.Error(codes.Internal, "list saved searches failed")
	}
	wire := make([]*searchv1.SavedSearch, 0, len(items))
	for _, s := range items {
		wire = append(wire, toWireSavedSearch(s))
	}
	next := ""
	if int64(from+size) < total {
		next = strconv.Itoa(from + size)
	}
	return &searchv1.ListSavedSearchesResponse{
		SavedSearches: wire,
		Page:          &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

// DeleteSavedSearch removes one of the caller's saved searches.
func (h *SearchHandler) DeleteSavedSearch(
	ctx context.Context,
	req *searchv1.DeleteSavedSearchRequest,
) (*searchv1.DeleteSavedSearchResponse, error) {
	userID, err := h.callerID(ctx, scopeSearchWrite)
	if err != nil {
		return nil, err
	}
	if h.saved == nil {
		return nil, status.Error(codes.Unimplemented, "saved searches not configured")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	if err := h.saved.Delete(ctx, req.GetId(), userID); err != nil {
		if errors.Is(err, repository.ErrSavedSearchNotFound) {
			return nil, status.Error(codes.NotFound, "saved search not found")
		}
		return nil, status.Error(codes.Internal, "delete saved search failed")
	}
	return &searchv1.DeleteSavedSearchResponse{}, nil
}

// RunSavedSearch loads a saved search owned by the caller and re-executes it
// through the SAME OpenSearch path as SearchListings, returning the same shape.
func (h *SearchHandler) RunSavedSearch(
	ctx context.Context,
	req *searchv1.RunSavedSearchRequest,
) (*searchv1.RunSavedSearchResponse, error) {
	userID, err := h.callerID(ctx, scopeSearchRead)
	if err != nil {
		return nil, err
	}
	if h.saved == nil {
		return nil, status.Error(codes.Unimplemented, "saved searches not configured")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	saved, err := h.saved.Get(ctx, req.GetId(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrSavedSearchNotFound) {
			return nil, status.Error(codes.NotFound, "saved search not found")
		}
		return nil, status.Error(codes.Internal, "load saved search failed")
	}
	filters, err := parseFilters(saved.FiltersJSON)
	if err != nil {
		// A row that somehow holds a malformed blob shouldn't 500 the run; treat
		// it as no structured filters and run the free-text query alone.
		filters = nil
	}
	// Re-apply the visibility policy at RUN time with the runner's identity (a saved
	// draft filter never outlives its owner's right to it), then reuse the
	// existing search path exactly (default first page).
	p, _ := interceptor.PrincipalFromContext(ctx)
	filters, err = effectiveFilters(p, filters)
	if err != nil {
		return nil, err
	}
	res, err := h.idx.Search(ctx, saved.Query, filters, "", 0, 0, 0, searchv1.SortBy_SORT_BY_UNSPECIFIED, 0, defaultPageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "search failed")
	}
	hits := make([]*searchv1.SearchHit, 0, len(res.Hits))
	for _, hit := range res.Hits {
		hits = append(hits, toWireHit(hit))
	}
	next := ""
	if int64(defaultPageSize) < res.Total {
		next = strconv.Itoa(defaultPageSize)
	}
	return &searchv1.RunSavedSearchResponse{
		Hits:   hits,
		Page:   &commonv1.PageResponse{NextCursor: next, Total: res.Total},
		Facets: toFacets(res.Facets),
	}, nil
}

// hasFilters reports whether filters_json carries at least one filter, so an
// empty "" or "{}" doesn't count as a savable filter set.
func hasFilters(filtersJSON string) bool {
	m, err := parseFilters(filtersJSON)
	return err == nil && len(m) > 0
}

// parseFilters decodes the opaque filters_json (mirroring
// SearchListingsRequest.filters) into the string->string term map the index
// expects. An empty string means "no filters".
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

func toWireSavedSearch(s repository.SavedSearch) *searchv1.SavedSearch {
	out := &searchv1.SavedSearch{
		Id:          s.ID,
		Query:       s.Query,
		FiltersJson: s.FiltersJSON,
	}
	if !s.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(s.CreatedAt)
	}
	return out
}
