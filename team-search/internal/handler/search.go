// Package handler is the gRPC transport adapter for SearchService: it maps
// between wire types and the OpenSearch read-model.
package handler

import (
	"context"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"

	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/interceptor"
	"github.com/buidangphuc/team-search/internal/repository"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

const (
	defaultPageSize = 10
	maxPageSize     = 50
	defaultSuggest  = 5
	maxSuggest      = 20
)

// SearchHandler implements searchv1.SearchServiceServer over an index.Index / retrieval.Engine for
// queries and a repository.SavedSearchRepository for user-owned saved searches.
type SearchHandler struct {
	searchv1.UnimplementedSearchServiceServer
	idx    index.Index
	engine *retrieval.Engine
	saved  repository.SavedSearchRepository
}

// NewSearchHandler builds the handler around the read-model index and the saved-search store.
func NewSearchHandler(idx index.Index, saved repository.SavedSearchRepository) *SearchHandler {
	return NewSearchHandlerWithEngine(idx, nil, saved)
}

// NewSearchHandlerWithEngine builds the handler with a retrieval engine for multi-strategy search.
func NewSearchHandlerWithEngine(idx index.Index, engine *retrieval.Engine, saved repository.SavedSearchRepository) *SearchHandler {
	return &SearchHandler{idx: idx, engine: engine, saved: saved}
}

// SearchListings runs a multi-strategy hybrid, semantic, or lexical query against the read-model.
func (h *SearchHandler) SearchListings(
	ctx context.Context,
	req *searchv1.SearchListingsRequest,
) (*searchv1.SearchListingsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "search:read"); err != nil {
		return nil, err
	}
	from, size := decodePage(req.GetPage())

	var res index.SearchResult
	var err error

	if h.engine != nil {
		res, _, err = h.engine.Execute(ctx, retrieval.SearchParams{
			Query:      req.GetQuery(),
			Filters:    req.GetFilters(),
			CategoryID: req.GetCategoryId(),
			MinPrice:   req.GetMinPrice(),
			MaxPrice:   req.GetMaxPrice(),
			MinRating:  req.GetMinRating(),
			SortBy:     req.GetSortBy(),
			SearchMode: req.GetSearchMode(),
			From:       from,
			Size:       size,
		})
	} else {
		res, err = h.idx.Search(ctx, req.GetQuery(), req.GetFilters(), req.GetCategoryId(), req.GetMinPrice(), req.GetMaxPrice(), req.GetMinRating(), req.GetSortBy(), from, size)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, "search failed")
	}
	hits := make([]*searchv1.SearchHit, 0, len(res.Hits))
	for _, hit := range res.Hits {
		hits = append(hits, &searchv1.SearchHit{ListingId: hit.ListingID, Score: float32(hit.Score)})
	}
	next := ""
	if int64(from+size) < res.Total {
		next = strconv.Itoa(from + size)
	}
	return &searchv1.SearchListingsResponse{
		Hits:   hits,
		Page:   &commonv1.PageResponse{NextCursor: next, Total: res.Total},
		Facets: toFacets(res.Facets),
	}, nil
}

// toFacets maps the index-layer facet counts to the wire Facets message. The
// buckets slices are always non-nil (the index guarantees it), so an empty
// result set yields empty — not nil — facet lists.
func toFacets(f index.Facets) *searchv1.Facets {
	return &searchv1.Facets{
		Categories:  toFacetBuckets(f.Categories),
		PriceRanges: toFacetBuckets(f.PriceRanges),
		Ratings:     toFacetBuckets(f.Ratings),
		Sellers:     toFacetBuckets(f.Sellers),
	}
}

func toFacetBuckets(buckets []index.FacetBucket) []*searchv1.FacetBucket {
	out := make([]*searchv1.FacetBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, &searchv1.FacetBucket{Key: b.Key, Count: b.Count})
	}
	return out
}

// Suggest returns type-ahead completions for a partial query.
func (h *SearchHandler) Suggest(
	ctx context.Context,
	req *searchv1.SuggestRequest,
) (*searchv1.SuggestResponse, error) {
	if err := interceptor.RequireScopes(ctx, "search:read"); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	switch {
	case limit <= 0:
		limit = defaultSuggest
	case limit > maxSuggest:
		limit = maxSuggest
	}
	suggestions, err := h.idx.Suggest(ctx, req.GetQuery(), limit)
	if err != nil {
		return nil, status.Error(codes.Internal, "suggest failed")
	}
	return &searchv1.SuggestResponse{Suggestions: suggestions}, nil
}

// decodePage turns the opaque cursor (an integer offset for the lexical engine)
// and page size into OpenSearch from/size, clamping the size.
func decodePage(p *commonv1.PageRequest) (from, size int) {
	size = defaultPageSize
	if p != nil {
		if ps := int(p.GetPageSize()); ps > 0 {
			size = ps
		}
		if c := p.GetCursor(); c != "" {
			if n, err := strconv.Atoi(c); err == nil && n >= 0 {
				from = n
			}
		}
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return from, size
}
