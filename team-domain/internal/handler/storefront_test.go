package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// newStorefrontHandler builds a ListingHandler wired with an in-memory
// storefront store (listing deps unused by the storefront RPCs).
func newStorefrontHandler() *handler.ListingHandler {
	sfSvc := service.NewStorefrontService(repository.NewInMemoryStorefrontRepository())
	return handler.NewListingHandler(service.NewListingService(repository.NewInMemoryListingRepository()), nil, nil).
		WithStorefront(sfSvc)
}

func sellerCtx(id string) context.Context {
	return contextWithPrincipal(context.Background(), &commonv1.Principal{
		Id:     id,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"listing.write", "listing.read"},
	})
}

// TestUpsertThenGetBySeller: happy path — a seller writes their storefront and
// reads it back by seller_id. Also asserts owner-scoping: a spoofed seller_id in
// the request body is ignored in favor of the authenticated principal.
func TestUpsertThenGetBySeller(t *testing.T) {
	h := newStorefrontHandler()
	ctx := sellerCtx("seller_a")

	upRes, err := h.UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{
			SellerId:           "SPOOFED", // must be overridden by the principal
			Slug:               "shop-a",
			BannerUrl:          "https://cdn/banner-a.png",
			Tagline:            "Hàng chính hãng",
			FeaturedListingIds: []string{"l1", "l2"},
			Theme:              "festive",
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if upRes.GetStorefront().GetSellerId() != "seller_a" {
		t.Errorf("owner-scope not enforced: got seller_id %q, want seller_a", upRes.GetStorefront().GetSellerId())
	}

	getRes, err := h.GetStorefront(ctx, &listingv1.GetStorefrontRequest{SellerId: "seller_a"})
	if err != nil {
		t.Fatalf("get by seller: %v", err)
	}
	got := getRes.GetStorefront()
	if got.GetSlug() != "shop-a" || got.GetTagline() != "Hàng chính hãng" || got.GetTheme() != "festive" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if len(got.GetFeaturedListingIds()) != 2 {
		t.Errorf("expected 2 featured ids, got %v", got.GetFeaturedListingIds())
	}

	// Upsert again replaces (no duplicate row / no self slug-conflict).
	if _, err := h.UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: "shop-a", Tagline: "Cập nhật"},
	}); err != nil {
		t.Fatalf("re-upsert same seller+slug should succeed: %v", err)
	}
}

// TestGetBySlug: a storefront is resolvable by its public slug when seller_id is
// omitted.
func TestGetBySlug(t *testing.T) {
	h := newStorefrontHandler()
	ctx := sellerCtx("seller_b")

	if _, err := h.UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: "cool-shop", Tagline: "Giá tốt"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	getRes, err := h.GetStorefront(ctx, &listingv1.GetStorefrontRequest{Slug: "cool-shop"})
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if getRes.GetStorefront().GetSellerId() != "seller_b" {
		t.Errorf("expected seller_b, got %q", getRes.GetStorefront().GetSellerId())
	}
}

// TestSlugUniquenessAcrossSellers: a slug already owned by one seller cannot be
// claimed by another (cross-user isolation) → AlreadyExists.
func TestSlugUniquenessAcrossSellers(t *testing.T) {
	h := newStorefrontHandler()

	if _, err := h.UpsertStorefront(sellerCtx("seller_a"), &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: "premium"},
	}); err != nil {
		t.Fatalf("seller_a upsert: %v", err)
	}

	_, err := h.UpsertStorefront(sellerCtx("seller_b"), &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: "premium"},
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists for taken slug, got %v", err)
	}
}

// TestStorefrontEdgeCases: empty input and anonymous callers are handled without
// panics and with the right gRPC codes.
func TestStorefrontEdgeCases(t *testing.T) {
	h := newStorefrontHandler()
	ctx := sellerCtx("seller_a")

	t.Run("upsert nil storefront", func(t *testing.T) {
		_, err := h.UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("upsert missing slug", func(t *testing.T) {
		_, err := h.UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{
			Storefront: &listingv1.Storefront{Tagline: "no slug"},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("get with neither key", func(t *testing.T) {
		_, err := h.GetStorefront(ctx, &listingv1.GetStorefrontRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("get missing storefront", func(t *testing.T) {
		_, err := h.GetStorefront(ctx, &listingv1.GetStorefrontRequest{SellerId: "nobody"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})

	t.Run("anonymous upsert denied", func(t *testing.T) {
		_, err := h.UpsertStorefront(context.Background(), &listingv1.UpsertStorefrontRequest{
			Storefront: &listingv1.Storefront{Slug: "x"},
		})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("want Unauthenticated, got %v", err)
		}
	})
}
