// This file implements the Seller Storefront RPCs (F5) on ListingHandler,
// mirroring the listing CRUD flow: transport-only, delegating to
// service.StorefrontService and mapping between wire (protobuf) and domain.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// UpsertStorefront creates or replaces the calling seller's storefront. It is
// owner-scoped: the row is always keyed by the authenticated principal, so the
// seller_id in the request body is ignored (a seller only edits their own shop).
func (h *ListingHandler) UpsertStorefront(
	ctx context.Context,
	req *listingv1.UpsertStorefrontRequest,
) (*listingv1.UpsertStorefrontResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if h.storefronts == nil {
		return nil, status.Error(codes.Unavailable, "storefront service unavailable")
	}
	if req.GetStorefront() == nil {
		return nil, status.Error(codes.InvalidArgument, "storefront is required")
	}
	ownerID, _ := principalOwner(ctx)

	saved, err := h.storefronts.Upsert(ctx, storefrontFromWire(req.GetStorefront()), ownerID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSlugRequired):
			return nil, status.Error(codes.InvalidArgument, "slug is required")
		case errors.Is(err, service.ErrDisplayNameInvalid):
			return nil, status.Error(codes.InvalidArgument, "invalid display_name")
		case errors.Is(err, service.ErrForbidden):
			return nil, status.Error(codes.PermissionDenied, "no authenticated seller")
		case errors.Is(err, repository.ErrSlugTaken):
			return nil, status.Error(codes.AlreadyExists, "slug already taken")
		default:
			return nil, status.Error(codes.Internal, "internal error")
		}
	}
	return &listingv1.UpsertStorefrontResponse{Storefront: storefrontToWire(saved)}, nil
}

// GetStorefront fetches a storefront by seller_id or slug (seller_id wins when
// both are set). Reads mirror GetListing: they require the listing.read scope.
func (h *ListingHandler) GetStorefront(
	ctx context.Context,
	req *listingv1.GetStorefrontRequest,
) (*listingv1.GetStorefrontResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.storefronts == nil {
		return nil, status.Error(codes.Unavailable, "storefront service unavailable")
	}
	if req.GetSellerId() == "" && req.GetSlug() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id or slug is required")
	}

	sf, err := h.storefronts.Get(ctx, req.GetSellerId(), req.GetSlug())
	if err != nil {
		if errors.Is(err, repository.ErrStorefrontNotFound) {
			return nil, status.Error(codes.NotFound, "not_found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.GetStorefrontResponse{Storefront: storefrontToWire(sf)}, nil
}

// BatchGetStorefronts resolves shop summaries for up to 100 seller ids in one
// store query. Same auth posture as GetStorefront (listing.read). Unknown
// sellers are omitted and duplicates ignored.
func (h *ListingHandler) BatchGetStorefronts(
	ctx context.Context,
	req *listingv1.BatchGetStorefrontsRequest,
) (*listingv1.BatchGetStorefrontsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.storefronts == nil {
		return nil, status.Error(codes.Unavailable, "storefront service unavailable")
	}
	found, err := h.storefronts.GetMany(ctx, req.GetSellerIds())
	if err != nil {
		if errors.Is(err, service.ErrTooManySellerIDs) {
			return nil, status.Error(codes.InvalidArgument, "too many seller_ids (max 100)")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	shops := make([]*listingv1.ShopSummary, 0, len(found))
	for _, sf := range found {
		shops = append(shops, &listingv1.ShopSummary{
			SellerId:    sf.SellerID,
			DisplayName: sf.DisplayName,
			Slug:        sf.Slug,
		})
	}
	return &listingv1.BatchGetStorefrontsResponse{Shops: shops}, nil
}

// storefrontFromWire maps the generated protobuf Storefront to the domain value.
// seller_id is intentionally carried through but the service overwrites it with
// the authenticated owner, so a spoofed body cannot target another seller.
func storefrontFromWire(s *listingv1.Storefront) repository.Storefront {
	return repository.Storefront{
		SellerID:           s.GetSellerId(),
		Slug:               s.GetSlug(),
		BannerURL:          s.GetBannerUrl(),
		Tagline:            s.GetTagline(),
		FeaturedListingIDs: s.GetFeaturedListingIds(),
		Theme:              s.GetTheme(),
		DisplayName:        s.GetDisplayName(),
	}
}

// storefrontToWire maps a domain Storefront to the generated protobuf message.
func storefrontToWire(s repository.Storefront) *listingv1.Storefront {
	return &listingv1.Storefront{
		SellerId:           s.SellerID,
		Slug:               s.Slug,
		BannerUrl:          s.BannerURL,
		Tagline:            s.Tagline,
		FeaturedListingIds: s.FeaturedListingIDs,
		Theme:              s.Theme,
		DisplayName:        s.DisplayName,
	}
}
