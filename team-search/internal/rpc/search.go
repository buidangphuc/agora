package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
	"github.com/buidangphuc/team-search/internal/store"
)

const (
	defaultPageSize = 10
	maxPageSize     = 50
	defaultSuggest  = 5
	maxSuggest      = 20
	maxMinRating    = 5
	filterMinRating = "min_rating"

	defaultRankerVersion = "v1"

	scopeSearchRead  = "search:read"
	scopeSearchWrite = "search:write"
)

type SavedSearches interface {
	Create(ctx context.Context, s store.SavedSearch) (store.SavedSearch, error)
	List(ctx context.Context, userID string, limit, offset int) ([]store.SavedSearch, int64, error)
	Get(ctx context.Context, id, userID string) (store.SavedSearch, error)
	Delete(ctx context.Context, id, userID string) error
}

type Handler struct {
	searchv1.UnimplementedSearchServiceServer
	idx    index.Index
	engine *retrieval.Engine
	saved  SavedSearches
	logger *slog.Logger
	errs   grpcx.ErrorTable

	rankerVersion string
}

type Option func(*Handler)

func WithRankerVersion(v string) Option {
	return func(h *Handler) {
		if v != "" {
			h.rankerVersion = v
		}
	}
}

func newRequestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return hex.EncodeToString(b[:]), nil
}

func NewHandler(idx index.Index, engine *retrieval.Engine, saved SavedSearches, logger *slog.Logger, opts ...Option) (*Handler, error) {
	if idx == nil {
		return nil, errors.New("rpc: index is required")
	}
	if saved == nil {
		return nil, errors.New("rpc: saved searches store is required")
	}
	errs, err := newErrorTable()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{idx: idx, engine: engine, saved: saved, logger: logger, errs: errs, rankerVersion: defaultRankerVersion}
	for _, o := range opts {
		o(h)
	}
	return h, nil
}

func (h *Handler) SearchListings(ctx context.Context, req *searchv1.SearchListingsRequest) (*searchv1.SearchListingsResponse, error) {
	p, err := auth.RequireScopesAllowAnonymous(ctx, scopeSearchRead)
	if err != nil {
		return nil, err
	}
	if err := validMinRating(req.GetMinRating()); err != nil {
		return nil, err
	}
	filters, err := effectiveFilters(p, req.GetFilters())
	if err != nil {
		return nil, err
	}
	from, size := decodePage(req.GetPage())

	var res index.SearchResult
	modelVersion := string(retrieval.ModeLexical)
	if h.engine != nil {
		var mode retrieval.Mode
		var reranked bool
		res, mode, reranked, err = h.engine.Execute(ctx, retrieval.SearchParams{
			Query:      req.GetQuery(),
			Filters:    filters,
			CategoryID: req.GetCategoryId(),
			MinPrice:   req.GetMinPrice(),
			MaxPrice:   req.GetMaxPrice(),
			MinRating:  req.GetMinRating(),
			SortBy:     req.GetSortBy(),
			SearchMode: req.GetSearchMode(),
			From:       from,
			Size:       size,
		})
		modelVersion = string(mode)
		if reranked {
			modelVersion += "+rerank"
		}
	} else {
		res, err = h.idx.Search(ctx, req.GetQuery(), filters, req.GetCategoryId(), req.GetMinPrice(), req.GetMaxPrice(), req.GetMinRating(), req.GetSortBy(), from, size)
	}
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	requestID, err := newRequestID()
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &searchv1.SearchListingsResponse{
		Hits:         toWireHits(res.Hits),
		Page:         &commonv1.PageResponse{NextCursor: nextCursor(from, size, res.Total), Total: res.Total},
		Facets:       toFacets(res.Facets),
		RequestId:    requestID,
		ModelVersion: modelVersion + "@" + h.rankerVersion,
	}, nil
}

func (h *Handler) Suggest(ctx context.Context, req *searchv1.SuggestRequest) (*searchv1.SuggestResponse, error) {
	if _, err := auth.RequireScopesAllowAnonymous(ctx, scopeSearchRead); err != nil {
		return nil, err
	}
	limit := pagination.Clamp(int(req.GetLimit()), defaultSuggest, maxSuggest)
	suggestions, err := h.idx.Suggest(ctx, req.GetQuery(), limit)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &searchv1.SuggestResponse{Suggestions: suggestions}, nil
}

func nextCursor(from, size int, total int64) string {
	if int64(from+size) < total {
		return strconv.Itoa(from + size)
	}
	return ""
}

func toWireHits(hits []index.Hit) []*searchv1.SearchHit {
	out := make([]*searchv1.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, toWireHit(hit))
	}
	return out
}

func toWireHit(hit index.Hit) *searchv1.SearchHit {
	out := &searchv1.SearchHit{ListingId: hit.ListingID, Score: float32(hit.Score)}
	if hit.Stock != nil {
		v := *hit.Stock
		out.Stock = &v
	}
	out.SoldCount = hit.SoldCount
	out.RatingAvg = hit.RatingAvg
	out.ReviewCount = hit.ReviewCount
	return out
}

func validMinRating(v int32) error {
	if v < 0 || v > maxMinRating {
		return status.Error(codes.InvalidArgument, "min_rating must be between 0 and 5")
	}
	return nil
}

func toFacets(f index.Facets) *searchv1.Facets {
	return &searchv1.Facets{
		Categories:  toFacetBuckets(f.Categories),
		PriceRanges: toFacetBuckets(f.PriceRanges),
		Ratings:     toFacetBuckets(f.Ratings),
		Sellers:     toFacetBuckets(f.Sellers),
		Tags:        toAttributeFacets(f.Tags),
		Skus:        toAttributeFacets(f.SKUs),
	}
}

func toAttributeFacets(groups []index.AttributeFacet) []*searchv1.AttributeFacet {
	out := make([]*searchv1.AttributeFacet, 0, len(groups))
	for _, g := range groups {
		out = append(out, &searchv1.AttributeFacet{Group: g.Group, Buckets: toFacetBuckets(g.Buckets)})
	}
	return out
}

func toFacetBuckets(buckets []index.FacetBucket) []*searchv1.FacetBucket {
	out := make([]*searchv1.FacetBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, &searchv1.FacetBucket{Key: b.Key, Count: b.Count})
	}
	return out
}

func decodePage(p *commonv1.PageRequest) (from, size int) {
	size = pagination.Clamp(int(p.GetPageSize()), defaultPageSize, maxPageSize)
	if n, err := strconv.Atoi(p.GetCursor()); err == nil && n >= 0 {
		from = n
	}
	return from, size
}
