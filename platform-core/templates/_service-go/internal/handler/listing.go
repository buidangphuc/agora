// Package handler is the gRPC transport adapter: it implements the generated
// service server interface and maps between wire (protobuf) and domain types.
//
// This file depends on generated code and only compiles AFTER `make proto`.
// See proto-vendor/README.md.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/your-org/team-service/generated/platform/common/v1"
	listingv1 "github.com/your-org/team-service/generated/platform/listing/v1"

	"github.com/your-org/team-service/internal/interceptor"
	"github.com/your-org/team-service/internal/repository"
	"github.com/your-org/team-service/internal/service"
)

// ListingHandler implements listingv1.ListingServiceServer.
type ListingHandler struct {
	// Embedding Unimplemented keeps the server forward-compatible when new RPCs
	// are added to the proto before they're implemented here.
	listingv1.UnimplementedListingServiceServer

	svc *service.ListingService
}

// NewListingHandler builds the handler around the business service.
func NewListingHandler(svc *service.ListingService) *ListingHandler {
	return &ListingHandler{svc: svc}
}

// GetListing returns a single listing. Seed implementation: it enforces a read
// scope, loads from the (in-memory) store, and maps domain -> wire.
func (h *ListingHandler) GetListing(
	ctx context.Context,
	req *listingv1.GetListingRequest,
) (*listingv1.GetListingResponse, error) {
	// Coarse RBAC via the Principal the auth interceptor placed on the context.
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	l, err := h.svc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "not_found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &listingv1.GetListingResponse{Listing: toWire(l)}, nil
}

// ListListings returns a page of listings.
func (h *ListingHandler) ListListings(
	ctx context.Context,
	req *listingv1.ListListingsRequest,
) (*listingv1.ListListingsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}

	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}

	items, next, err := h.svc.List(ctx, cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	wire := make([]*listingv1.Listing, 0, len(items))
	for _, l := range items {
		wire = append(wire, toWire(l))
	}
	return &listingv1.ListListingsResponse{
		Listings: wire,
		Page:     &commonv1.PageResponse{NextCursor: next, Total: int64(len(wire))},
	}, nil
}

// toWire maps a domain Listing to the generated protobuf message.
func toWire(l repository.Listing) *listingv1.Listing {
	return &listingv1.Listing{
		Id:          l.ID,
		Title:       l.Title,
		Description: l.Description,
		Price:       l.Price,
		Currency:    l.Currency,
		Status:      wireStatus(l.Status),
	}
}

func wireStatus(s string) listingv1.ListingStatus {
	switch s {
	case "draft":
		return listingv1.ListingStatus_LISTING_STATUS_DRAFT
	case "published":
		return listingv1.ListingStatus_LISTING_STATUS_PUBLISHED
	case "rejected":
		return listingv1.ListingStatus_LISTING_STATUS_REJECTED
	default:
		return listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED
	}
}
