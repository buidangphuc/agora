// Package handler is the gRPC transport adapter: it implements the generated
// service server interface and maps between wire (protobuf) and domain types.
//
// This file depends on generated code and only compiles AFTER `make proto`.
package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"

	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
	"github.com/buidangphuc/team-domain/internal/storage"
)

// ListingHandler implements listingv1.ListingServiceServer.
type ListingHandler struct {
	// Embedding Unimplemented keeps the server forward-compatible when new RPCs
	// are added to the proto before they're implemented here.
	listingv1.UnimplementedListingServiceServer

	svc        *service.ListingService
	categories repository.CategoryRepository
	storage    storage.StorageClient
	// storefronts backs the Seller Storefront RPCs (F5). Optional: nil until
	// wired via WithStorefront, in which case those RPCs report Unavailable.
	storefronts *service.StorefrontService
	// bundles backs the Listing Bundle RPCs (F5). Optional: nil until wired via
	// WithBundles, in which case those RPCs report Unavailable.
	bundles *service.BundleService
}

// WithBundles attaches the bundle service (F5) and returns the handler for
// chaining. Kept separate from NewListingHandler so the existing listing
// construction path (and its callers/tests) stay untouched.
func (h *ListingHandler) WithBundles(b *service.BundleService) *ListingHandler {
	h.bundles = b
	return h
}

// WithStorefront attaches the storefront service (F5) and returns the handler
// for chaining. Kept separate from NewListingHandler so the existing listing
// construction path (and its callers/tests) stay untouched.
func (h *ListingHandler) WithStorefront(sf *service.StorefrontService) *ListingHandler {
	h.storefronts = sf
	return h
}

// NewListingHandler builds the handler around the business service, category
// repo, and storage client. The handler no longer publishes to Kafka on the
// request path (the classic dual-write hazard): it builds the EventEnvelope and
// hands it to the service, which records it in the same transaction as the
// listing write (the transactional outbox, ADR-0002). A background relayer
// publishes those rows — see internal/events/relayer.go.
func NewListingHandler(
	svc *service.ListingService,
	categories repository.CategoryRepository,
	store storage.StorageClient,
) *ListingHandler {
	return &ListingHandler{
		svc:        svc,
		categories: categories,
		storage:    store,
	}
}

// GetListing returns a single listing: enforce read scope, load, map domain->wire.
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

// ListListings returns a keyset page of listings.
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

	page, err := h.svc.List(ctx, cursor, pageSize, req.GetStatus())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.ListListingsResponse{
		Listings: toWireList(page.Items),
		Page:     &commonv1.PageResponse{NextCursor: page.NextCursor, Total: page.Total},
	}, nil
}

// ListMyListings returns the caller's own listings (the seller center).
func (h *ListingHandler) ListMyListings(
	ctx context.Context,
	req *listingv1.ListMyListingsRequest,
) (*listingv1.ListMyListingsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	ownerID, _ := principalOwner(ctx)

	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}

	page, err := h.svc.ListMine(ctx, ownerID, cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.ListMyListingsResponse{
		Listings: toWireList(page.Items),
		Page:     &commonv1.PageResponse{NextCursor: page.NextCursor, Total: page.Total},
	}, nil
}

// CreateListing stores a new listing (seller "đăng bán") and emits a
// ListingChanged(CREATED) event so read-models (team-search) can index it.
func (h *ListingHandler) CreateListing(
	ctx context.Context,
	req *listingv1.CreateListingRequest,
) (*listingv1.CreateListingResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetListing() == nil {
		return nil, status.Error(codes.InvalidArgument, "listing is required")
	}
	ownerID, _ := principalOwner(ctx)
	created, err := h.svc.CreateWithEvent(ctx, fromWire(req.GetListing()), ownerID,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_CREATED))
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &listingv1.CreateListingResponse{Listing: toWire(created)}, nil
}

// UpdateListing replaces an existing listing and emits ListingChanged(UPDATED).
func (h *ListingHandler) UpdateListing(
	ctx context.Context,
	req *listingv1.UpdateListingRequest,
) (*listingv1.UpdateListingResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetListing() == nil || req.GetListing().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing.id is required")
	}
	ownerID, isAdmin := principalOwner(ctx)
	updated, err := h.svc.UpdateWithEvent(ctx, fromWire(req.GetListing()), ownerID, isAdmin,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, "not_found")
		case errors.Is(err, service.ErrForbidden):
			return nil, status.Error(codes.PermissionDenied, "not the listing owner")
		default:
			return nil, status.Error(codes.Internal, "internal error")
		}
	}
	return &listingv1.UpdateListingResponse{Listing: toWire(updated)}, nil
}

// DeleteListing removes the caller's own listing and emits ListingChanged(DELETED).
func (h *ListingHandler) DeleteListing(
	ctx context.Context,
	req *listingv1.DeleteListingRequest,
) (*listingv1.DeleteListingResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ownerID, isAdmin := principalOwner(ctx)
	_, err := h.svc.DeleteWithEvent(ctx, req.GetId(), ownerID, isAdmin,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_DELETED))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, "not_found")
		case errors.Is(err, service.ErrForbidden):
			return nil, status.Error(codes.PermissionDenied, "not the listing owner")
		default:
			return nil, status.Error(codes.Internal, "internal error")
		}
	}
	return &listingv1.DeleteListingResponse{}, nil
}

// GetImageUploadUrl returns a presigned S3 PUT URL for uploading a product image.
func (h *ListingHandler) GetImageUploadUrl(
	ctx context.Context,
	req *listingv1.GetImageUploadUrlRequest,
) (*listingv1.GetImageUploadUrlResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if h.storage == nil {
		return nil, status.Error(codes.Unavailable, "storage service unavailable")
	}
	uploadURL, imageKey, publicURL, err := h.storage.GetPresignedUploadURL(
		ctx,
		req.GetContentType(),
		req.GetFilename(),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate upload url: %v", err)
	}
	return &listingv1.GetImageUploadUrlResponse{
		UploadUrl: uploadURL,
		ImageKey:  imageKey,
		PublicUrl: publicURL,
	}, nil
}

// ListCategories returns product categories, optionally filtered by parent_id.
func (h *ListingHandler) ListCategories(
	ctx context.Context,
	req *listingv1.ListCategoriesRequest,
) (*listingv1.ListCategoriesResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.categories == nil {
		return &listingv1.ListCategoriesResponse{}, nil
	}
	items, err := h.categories.List(ctx, req.GetParentId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list categories: %v", err)
	}
	wire := make([]*listingv1.Category, 0, len(items))
	for _, c := range items {
		wire = append(wire, &listingv1.Category{
			Id:           c.ID,
			Name:         c.Name,
			Slug:         c.Slug,
			ParentId:     c.ParentID,
			DisplayOrder: c.DisplayOrder,
			IconUrl:      c.IconURL,
		})
	}
	return &listingv1.ListCategoriesResponse{Categories: wire}, nil
}

// GetCategory returns a single category by id.
func (h *ListingHandler) GetCategory(
	ctx context.Context,
	req *listingv1.GetCategoryRequest,
) (*listingv1.GetCategoryResponse, error) {
	if err := interceptor.RequireScopes(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if h.categories == nil {
		return nil, status.Error(codes.NotFound, "category not found")
	}
	c, err := h.categories.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrCategoryNotFound) {
			return nil, status.Error(codes.NotFound, "category not found")
		}
		return nil, status.Errorf(codes.Internal, "get category: %v", err)
	}
	return &listingv1.GetCategoryResponse{
		Category: &listingv1.Category{
			Id:           c.ID,
			Name:         c.Name,
			Slug:         c.Slug,
			ParentId:     c.ParentID,
			DisplayOrder: c.DisplayOrder,
			IconUrl:      c.IconURL,
		},
	}, nil
}

// ReserveStock atomically decrements inventory if sufficient stock is available.
func (h *ListingHandler) ReserveStock(
	ctx context.Context,
	req *listingv1.ReserveStockRequest,
) (*listingv1.ReserveStockResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be > 0")
	}
	// Idempotent on the caller-supplied reservation_id (AD5): a retried checkout
	// with the same id is a no-op, so stock is decremented exactly once. An empty
	// id falls back to a plain (non-idempotent) reserve in the service layer.
	if err := h.svc.ReserveStockIdempotent(ctx, req.GetReservationId(), req.GetListingId(), req.GetVariantId(), req.GetQuantity()); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "listing not found")
		}
		if errors.Is(err, repository.ErrVariantNotFound) {
			return nil, status.Error(codes.NotFound, "variant not found")
		}
		if errors.Is(err, repository.ErrOutOfStock) {
			return &listingv1.ReserveStockResponse{
				Success: false,
				Message: "insufficient stock",
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "reserve stock: %v", err)
	}
	return &listingv1.ReserveStockResponse{Success: true}, nil
}

// ReleaseStock releases previously reserved inventory back into stock.
func (h *ListingHandler) ReleaseStock(
	ctx context.Context,
	req *listingv1.ReleaseStockRequest,
) (*listingv1.ReleaseStockResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be > 0")
	}
	if err := h.svc.ReleaseStock(ctx, req.GetListingId(), req.GetVariantId(), req.GetQuantity()); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "listing not found")
		}
		if errors.Is(err, repository.ErrVariantNotFound) {
			return nil, status.Error(codes.NotFound, "variant not found")
		}
		return nil, status.Errorf(codes.Internal, "release stock: %v", err)
	}
	return &listingv1.ReleaseStockResponse{Success: true}, nil
}

// enqueueListingChanged returns the EnqueueFn the service invokes inside the
// write transaction. It captures the acting principal and request id from the
// context now (request-scoped), and — given the just-persisted listing — builds
// the ListingChanged EventEnvelope with a fresh event_id that becomes the outbox
// row's primary key (a stable dedupe id under at-least-once). No Kafka produce
// happens here: the row is written in the same tx as the listing, and the
// relayer publishes it later.
func (h *ListingHandler) enqueueListingChanged(ctx context.Context, change listingv1.ChangeType) repository.EnqueueFn {
	var principal *commonv1.Principal
	if p, ok := interceptor.PrincipalFromContext(ctx); ok {
		principal = p
	}
	requestID, _ := interceptor.RequestIDFromContext(ctx)
	return func(written repository.Listing) (repository.OutboxRow, error) {
		wire := toWire(written)
		eventID := uuid.NewString()
		payload, err := events.BuildListingChangedEnvelope(eventID, wire, change, principal, requestID)
		if err != nil {
			return repository.OutboxRow{}, err
		}
		return repository.OutboxRow{
			EventID:       eventID,
			AggregateType: "Listing",
			AggregateID:   written.ID,
			EventType:     events.ListingChangedEventType,
			Payload:       payload,
			RequestID:     requestID,
		}, nil
	}
}

// principalOwner extracts the owner id + admin flag from the context Principal.
func principalOwner(ctx context.Context) (id string, isAdmin bool) {
	p, ok := interceptor.PrincipalFromContext(ctx)
	if !ok {
		return "", false
	}
	for _, s := range p.GetScopes() {
		if s == "admin" {
			isAdmin = true
		}
	}
	return p.GetId(), isAdmin
}

// fromWire maps a generated protobuf Listing to the domain value.
func fromWire(l *listingv1.Listing) repository.Listing {
	variants := make([]repository.Variant, 0, len(l.GetVariants()))
	for _, v := range l.GetVariants() {
		variants = append(variants, repository.Variant{
			ID:        v.GetId(),
			ListingID: l.GetId(),
			Name:      v.GetName(),
			SKU:       v.GetSku(),
			Price:     v.GetPrice(),
			Stock:     v.GetStock(),
			ImageURL:  v.GetImageUrl(),
		})
	}
	return repository.Listing{
		ID:          l.GetId(),
		Title:       l.GetTitle(),
		Description: l.GetDescription(),
		Price:       l.GetPrice(),
		Currency:    l.GetCurrency(),
		Status:      domainStatus(l.GetStatus()),
		SellerID:    l.GetSellerId(),
		ImageKeys:   l.GetImageKeys(),
		CategoryID:  l.GetCategoryId(),
		Stock:       l.GetStock(),
		Variants:    variants,
	}
}

// domainStatus maps the proto enum to the stored string.
func domainStatus(s listingv1.ListingStatus) string {
	switch s {
	case listingv1.ListingStatus_LISTING_STATUS_DRAFT:
		return "draft"
	case listingv1.ListingStatus_LISTING_STATUS_PUBLISHED:
		return "published"
	case listingv1.ListingStatus_LISTING_STATUS_REJECTED:
		return "rejected"
	default:
		return "draft"
	}
}

// toWire maps a domain Listing to the generated protobuf message.
func toWire(l repository.Listing) *listingv1.Listing {
	variants := make([]*listingv1.Variant, 0, len(l.Variants))
	for _, v := range l.Variants {
		variants = append(variants, &listingv1.Variant{
			Id:        v.ID,
			ListingId: v.ListingID,
			Name:      v.Name,
			Sku:       v.SKU,
			Price:     v.Price,
			Stock:     v.Stock,
			ImageUrl:  v.ImageURL,
		})
	}
	return &listingv1.Listing{
		Id:          l.ID,
		Title:       l.Title,
		Description: l.Description,
		Price:       l.Price,
		Currency:    l.Currency,
		Status:      wireStatus(l.Status),
		SellerId:    l.SellerID,
		ImageKeys:   l.ImageKeys,
		CategoryId:  l.CategoryID,
		Stock:       l.Stock,
		Variants:    variants,
	}
}

// toWireList maps a slice of domain listings to wire messages.
func toWireList(items []repository.Listing) []*listingv1.Listing {
	wire := make([]*listingv1.Listing, 0, len(items))
	for _, l := range items {
		wire = append(wire, toWire(l))
	}
	return wire
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
