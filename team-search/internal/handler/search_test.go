package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/interceptor"
	"github.com/buidangphuc/team-search/internal/repository"
)

type mockIndex struct {
	docs          []index.ListingDoc
	lastMinRating int32
}

func (m *mockIndex) EnsureIndex(ctx context.Context) error { return nil }
func (m *mockIndex) Upsert(ctx context.Context, doc index.ListingDoc) error {
	m.docs = append(m.docs, doc)
	return nil
}
func (m *mockIndex) PartialUpdate(ctx context.Context, id string, partialDoc map[string]interface{}) error {
	return nil
}
func (m *mockIndex) Delete(ctx context.Context, id string) error { return nil }
func (m *mockIndex) Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	m.lastMinRating = minRating
	return index.SearchResult{
		Hits: []index.Hit{
			{ListingID: "listing_1", Score: 1.5},
			{ListingID: "listing_2", Score: 1.2},
		},
		Total: 2,
		Facets: index.Facets{
			Categories:  []index.FacetBucket{{Key: "cat_phones", Count: 2}},
			PriceRanges: []index.FacetBucket{{Key: "0-100000", Count: 1}, {Key: "1000000+", Count: 1}},
			Ratings:     []index.FacetBucket{{Key: "4", Count: 2}, {Key: "3", Count: 2}},
			Sellers:     []index.FacetBucket{{Key: "seller_1", Count: 2}},
		},
	}, nil
}
func (m *mockIndex) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	return []string{"iPhone 15", "iPhone 15 Pro", "iPhone 15 Pro Max"}, nil
}

// contextWithPrincipal injects a resolved Principal the way the gateway does:
// as x-principal-* metadata run through the service's unary interceptor. The
// interceptor's context setter is unexported, so tests reach it via this path.
func contextWithPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	md := metadata.Pairs(
		"x-principal-id", p.GetId(),
		"x-principal-type", "user",
		"x-principal-scopes", strings.Join(p.GetScopes(), ","),
	)
	ctx = metadata.NewIncomingContext(ctx, md)
	var out context.Context
	_, _ = interceptor.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) {
			out = c
			return nil, nil
		})
	return out
}

func TestSearchHandler(t *testing.T) {
	h := handler.NewSearchHandler(&mockIndex{}, repository.NewInMemorySavedSearchRepository())
	principal := &commonv1.Principal{
		Id:     "buyer_1",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"search:read"},
	}
	ctx := contextWithPrincipal(context.Background(), principal)

	t.Run("SearchListings", func(t *testing.T) {
		res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{
			Query: "iPhone",
		})
		if err != nil {
			t.Fatalf("unexpected error searching: %v", err)
		}
		if len(res.Hits) != 2 {
			t.Errorf("expected 2 hits, got %d", len(res.Hits))
		}
		if res.Page.Total != 2 {
			t.Errorf("expected total 2, got %d", res.Page.Total)
		}
	})

	t.Run("SearchListings maps facets", func(t *testing.T) {
		res, err := h.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "iPhone"})
		if err != nil {
			t.Fatalf("unexpected error searching: %v", err)
		}
		f := res.GetFacets()
		if f == nil {
			t.Fatal("expected non-nil facets")
		}
		if len(f.GetCategories()) != 1 || f.GetCategories()[0].GetKey() != "cat_phones" || f.GetCategories()[0].GetCount() != 2 {
			t.Errorf("categories facet mismatch: %+v", f.GetCategories())
		}
		if len(f.GetPriceRanges()) != 2 || f.GetPriceRanges()[0].GetKey() != "0-100000" {
			t.Errorf("price_ranges facet mismatch: %+v", f.GetPriceRanges())
		}
		if len(f.GetRatings()) != 2 || f.GetRatings()[0].GetKey() != "4" || f.GetRatings()[0].GetCount() != 2 {
			t.Errorf("ratings facet mismatch: %+v", f.GetRatings())
		}
		if len(f.GetSellers()) != 1 || f.GetSellers()[0].GetKey() != "seller_1" {
			t.Errorf("sellers facet mismatch: %+v", f.GetSellers())
		}
	})

	t.Run("SearchListings forwards min_rating", func(t *testing.T) {
		mi := &mockIndex{}
		h2 := handler.NewSearchHandler(mi, repository.NewInMemorySavedSearchRepository())
		if _, err := h2.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x", MinRating: 4}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mi.lastMinRating != 4 {
			t.Errorf("expected min_rating 4 forwarded to index, got %d", mi.lastMinRating)
		}
	})

	t.Run("Suggest", func(t *testing.T) {
		res, err := h.Suggest(ctx, &searchv1.SuggestRequest{
			Query: "iPh",
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("unexpected error suggesting: %v", err)
		}
		if len(res.Suggestions) != 3 {
			t.Errorf("expected 3 suggestions, got %d", len(res.Suggestions))
		}
	})
}
