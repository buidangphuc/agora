package handler_test

import (
	"context"
	"fmt"
	"strings"
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

func upsertName(t *testing.T, h *handler.ListingHandler, seller, slug, name string) error {
	t.Helper()
	_, err := h.UpsertStorefront(sellerCtx(seller), &listingv1.UpsertStorefrontRequest{
		Storefront: &listingv1.Storefront{Slug: slug, DisplayName: name},
	})
	return err
}

// TestDisplayNameValidation: trim, length cap (runes), control chars, empty and
// spoofed seller_id; a rejected write leaves the stored name unchanged.
func TestDisplayNameValidation(t *testing.T) {
	h := newStorefrontHandler()

	if err := upsertName(t, h, "seller_a", "shop-a", "  Tiem Hoa Nho  "); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := h.GetStorefront(sellerCtx("visitor"), &listingv1.GetStorefrontRequest{SellerId: "seller_a"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetStorefront().GetDisplayName() != "Tiem Hoa Nho" {
		t.Errorf("name not trimmed/returned: %q", got.GetStorefront().GetDisplayName())
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"80 runes ok", strings.Repeat("ế", 80), false},
		{"81 runes rejected", strings.Repeat("a", 81), true},
		{"control char rejected", "bad\x00name", true},
		{"newline rejected", "bad\nname", true},
		{"empty allowed", "", false},
		{"whitespace only trims to empty", "   ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newStorefrontHandler()
			err := upsertName(t, h, "seller_a", "shop-a", tc.input)
			if tc.wantErr && status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("rejected write keeps old name", func(t *testing.T) {
		h := newStorefrontHandler()
		_ = upsertName(t, h, "seller_a", "shop-a", "Old")
		if err := upsertName(t, h, "seller_a", "shop-a", strings.Repeat("a", 81)); err == nil {
			t.Fatal("expected error")
		}
		res, _ := h.GetStorefront(sellerCtx("seller_a"), &listingv1.GetStorefrontRequest{SellerId: "seller_a"})
		if res.GetStorefront().GetDisplayName() != "Old" {
			t.Errorf("name changed: %q", res.GetStorefront().GetDisplayName())
		}
	})

	t.Run("name change reflected", func(t *testing.T) {
		h := newStorefrontHandler()
		_ = upsertName(t, h, "seller_a", "shop-a", "Tiem Hoa Nho")
		_ = upsertName(t, h, "seller_a", "shop-a", "Tiem Hoa Lon")
		res, _ := h.GetStorefront(sellerCtx("seller_a"), &listingv1.GetStorefrontRequest{SellerId: "seller_a"})
		if res.GetStorefront().GetDisplayName() != "Tiem Hoa Lon" {
			t.Errorf("rename not reflected: %q", res.GetStorefront().GetDisplayName())
		}
	})

	t.Run("spoofed seller_id cannot set another seller's name", func(t *testing.T) {
		h := newStorefrontHandler()
		_ = upsertName(t, h, "seller_b", "shop-b", "Bee")
		_, err := h.UpsertStorefront(sellerCtx("seller_a"), &listingv1.UpsertStorefrontRequest{
			Storefront: &listingv1.Storefront{SellerId: "seller_b", Slug: "shop-a", DisplayName: "Hijack"},
		})
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		b, _ := h.GetStorefront(sellerCtx("x"), &listingv1.GetStorefrontRequest{SellerId: "seller_b"})
		a, _ := h.GetStorefront(sellerCtx("x"), &listingv1.GetStorefrontRequest{SellerId: "seller_a"})
		if b.GetStorefront().GetDisplayName() != "Bee" || a.GetStorefront().GetDisplayName() != "Hijack" {
			t.Errorf("owner scoping broken: a=%q b=%q", a.GetStorefront().GetDisplayName(), b.GetStorefront().GetDisplayName())
		}
	})
}

// countingRepo wraps the in-memory repo to count batch queries.
type countingRepo struct {
	*repository.InMemoryStorefrontRepository
	batchCalls int
}

func (c *countingRepo) GetBySellers(ctx context.Context, ids []string) ([]repository.Storefront, error) {
	c.batchCalls++
	return c.InMemoryStorefrontRepository.GetBySellers(ctx, ids)
}

func TestBatchGetStorefronts(t *testing.T) {
	repo := &countingRepo{InMemoryStorefrontRepository: repository.NewInMemoryStorefrontRepository()}
	h := handler.NewListingHandler(service.NewListingService(repository.NewInMemoryListingRepository()), nil, nil).
		WithStorefront(service.NewStorefrontService(repo))
	_ = upsertName(t, h, "seller_a", "shop-a", "Shop Alpha")
	_ = upsertName(t, h, "seller_b", "shop-b", "Shop Beta")

	call := func(ctx context.Context, ids ...string) (*listingv1.BatchGetStorefrontsResponse, error) {
		return h.BatchGetStorefronts(ctx, &listingv1.BatchGetStorefrontsRequest{SellerIds: ids})
	}
	visitor := contextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS, Scopes: []string{"listing.read"},
	})

	t.Run("two sellers, one query", func(t *testing.T) {
		repo.batchCalls = 0
		res, err := call(visitor, "seller_a", "seller_b")
		if err != nil {
			t.Fatalf("batch: %v", err)
		}
		names := map[string]string{}
		for _, s := range res.GetShops() {
			names[s.GetSellerId()] = s.GetDisplayName()
		}
		if len(names) != 2 || names["seller_a"] != "Shop Alpha" || names["seller_b"] != "Shop Beta" {
			t.Errorf("unexpected shops: %v", names)
		}
		if repo.batchCalls != 1 {
			t.Errorf("want exactly 1 store query, got %d", repo.batchCalls)
		}
	})

	t.Run("unknown seller omitted", func(t *testing.T) {
		res, err := call(visitor, "seller_a", "seller_z")
		if err != nil {
			t.Fatalf("batch: %v", err)
		}
		if len(res.GetShops()) != 1 || res.GetShops()[0].GetSellerId() != "seller_a" {
			t.Errorf("want only seller_a, got %v", res.GetShops())
		}
	})

	t.Run("duplicates ignored", func(t *testing.T) {
		res, err := call(visitor, "seller_a", "seller_a", "seller_a")
		if err != nil || len(res.GetShops()) != 1 {
			t.Errorf("want 1 entry, got %v err %v", res.GetShops(), err)
		}
	})

	t.Run("101 ids rejected, 100 ok", func(t *testing.T) {
		ids := make([]string, 101)
		for i := range ids {
			ids[i] = fmt.Sprintf("s%03d", i)
		}
		if _, err := call(visitor, ids...); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("101 ids: want InvalidArgument, got %v", err)
		}
		if _, err := call(visitor, ids[:100]...); err != nil {
			t.Fatalf("100 ids: %v", err)
		}
	})

	t.Run("requires listing.read like GetStorefront", func(t *testing.T) {
		noScope := contextWithPrincipal(context.Background(), &commonv1.Principal{Id: "u", Scopes: []string{"search:read"}})
		if _, err := call(noScope, "seller_a"); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
		if _, err := call(context.Background(), "seller_a"); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("want Unauthenticated, got %v", err)
		}
	})
}
