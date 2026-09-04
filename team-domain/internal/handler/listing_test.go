package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

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

func TestListingHandler(t *testing.T) {
	repo := repository.NewInMemoryListingRepository()
	svc := service.NewListingService(repo)
	h := handler.NewListingHandler(svc, nil, nil)

	principal := &commonv1.Principal{
		Id:     "seller_test",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"listing.write", "listing.read"},
	}
	ctx := contextWithPrincipal(context.Background(), principal)

	t.Run("Create and Get Listing via Handler", func(t *testing.T) {
		createRes, err := h.CreateListing(ctx, &listingv1.CreateListingRequest{
			Listing: &listingv1.Listing{
				Title:       "MacBook Pro M3",
				Description: "Laptop",
				Price:       45000000,
				Currency:    "VND",
				Status:      listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
				Stock:       5,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error creating listing: %v", err)
		}
		if createRes.Listing.Title != "MacBook Pro M3" {
			t.Errorf("expected MacBook Pro M3, got %s", createRes.Listing.Title)
		}

		getRes, err := h.GetListing(ctx, &listingv1.GetListingRequest{
			Id: createRes.Listing.Id,
		})
		if err != nil {
			t.Fatalf("unexpected error getting listing: %v", err)
		}
		if getRes.Listing.Id != createRes.Listing.Id {
			t.Errorf("expected ID %s, got %s", createRes.Listing.Id, getRes.Listing.Id)
		}

		listRes, err := h.ListListings(ctx, &listingv1.ListListingsRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error listing: %v", err)
		}
		if len(listRes.Listings) == 0 {
			t.Errorf("expected non-empty listings")
		}
	})
}
