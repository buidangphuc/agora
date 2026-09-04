// This file implements the Listing Bundle RPCs (F5) on ListingHandler,
// mirroring the listing CRUD flow: transport-only, delegating to
// service.BundleService and mapping between wire (protobuf) and domain.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// CreateBundle stores a new bundle for the calling seller. It is owner-scoped:
// the row's seller_id is always the authenticated principal, so a seller only
// ever creates bundles under their own account.
func (h *ListingHandler) CreateBundle(
	ctx context.Context,
	req *listingv1.CreateBundleRequest,
) (*listingv1.CreateBundleResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if h.bundles == nil {
		return nil, status.Error(codes.Unavailable, "bundle service unavailable")
	}
	ownerID, _ := principalOwner(ctx)

	created, err := h.bundles.Create(ctx, repository.Bundle{
		Title:       req.GetTitle(),
		ListingIDs:  req.GetListingIds(),
		BundlePrice: req.GetBundlePrice(),
	}, ownerID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTitleRequired):
			return nil, status.Error(codes.InvalidArgument, "title is required")
		case errors.Is(err, service.ErrForbidden):
			return nil, status.Error(codes.PermissionDenied, "no authenticated seller")
		default:
			return nil, status.Error(codes.Internal, "internal error")
		}
	}
	return &listingv1.CreateBundleResponse{Bundle: bundleToWire(created)}, nil
}

// GetBundle returns a single bundle by id. Reads mirror GetListing: they require
// the listing.read scope.
func (h *ListingHandler) GetBundle(
	ctx context.Context,
	req *listingv1.GetBundleRequest,
) (*listingv1.GetBundleResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.bundles == nil {
		return nil, status.Error(codes.Unavailable, "bundle service unavailable")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	b, err := h.bundles.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrBundleNotFound) {
			return nil, status.Error(codes.NotFound, "not_found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.GetBundleResponse{Bundle: bundleToWire(b)}, nil
}

// ListBundlesBySeller returns a keyset page of a seller's bundles.
func (h *ListingHandler) ListBundlesBySeller(
	ctx context.Context,
	req *listingv1.ListBundlesBySellerRequest,
) (*listingv1.ListBundlesBySellerResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.bundles == nil {
		return nil, status.Error(codes.Unavailable, "bundle service unavailable")
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}

	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}

	page, err := h.bundles.ListBySeller(ctx, req.GetSellerId(), cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.ListBundlesBySellerResponse{
		Bundles: bundlesToWire(page.Items),
		Page:    &commonv1.PageResponse{NextCursor: page.NextCursor, Total: page.Total},
	}, nil
}

// bundleToWire maps a domain Bundle to the generated protobuf message.
func bundleToWire(b repository.Bundle) *listingv1.Bundle {
	out := &listingv1.Bundle{
		Id:          b.ID,
		SellerId:    b.SellerID,
		Title:       b.Title,
		ListingIds:  b.ListingIDs,
		BundlePrice: b.BundlePrice,
	}
	if !b.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(b.CreatedAt)
	}
	return out
}

// bundlesToWire maps a slice of domain bundles to wire messages.
func bundlesToWire(items []repository.Bundle) []*listingv1.Bundle {
	wire := make([]*listingv1.Bundle, 0, len(items))
	for _, b := range items {
		wire = append(wire, bundleToWire(b))
	}
	return wire
}
