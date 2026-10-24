// Package rpc is the gRPC transport adapter: it implements the generated
// service server interface and maps between wire (protobuf) and domain types.
//
// This file depends on generated code and only compiles AFTER `make proto`.
package rpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"

	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/objects"
	"github.com/buidangphuc/team-domain/internal/store"
)

// ListingHandler implements listingv1.ListingServiceServer.
type ListingHandler struct {
	// Embedding Unimplemented keeps the server forward-compatible when new RPCs
	// are added to the proto before they're implemented here.
	listingv1.UnimplementedListingServiceServer

	svc        *listing.ListingService
	categories store.CategoryRepository
	storage    objects.StorageClient
	// storefronts backs the Seller Storefront RPCs (F5). Optional: nil until
	// wired via WithStorefront, in which case those RPCs report Unavailable.
	storefronts *listing.StorefrontService
	// bundles backs the Listing Bundle RPCs (F5). Optional: nil until wired via
	// WithBundles, in which case those RPCs report Unavailable.
	bundles *listing.BundleService
}

// WithBundles attaches the bundle service (F5) and returns the handler for
// chaining. Kept separate from NewListingHandler so the existing listing
// construction path (and its callers/tests) stay untouched.
func (h *ListingHandler) WithBundles(b *listing.BundleService) *ListingHandler {
	h.bundles = b
	return h
}

// WithStorefront attaches the storefront service (F5) and returns the handler
// for chaining. Kept separate from NewListingHandler so the existing listing
// construction path (and its callers/tests) stay untouched.
func (h *ListingHandler) WithStorefront(sf *listing.StorefrontService) *ListingHandler {
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
	svc *listing.ListingService,
	categories store.CategoryRepository,
	store objects.StorageClient,
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
	if _, err := auth.RequireScopesAllowAnonymous(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	l, err := h.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, fail(ctx, err)
	}

	// Non-published (draft, rejected) listings are visible only to the owner,
	// admin or an internal service; everyone else gets the same NOT_FOUND as an
	// unknown id so the response is not an existence oracle.
	if l.Status != statusPublished && !canSeeNonPublished(ctx, l.SellerID) {
		return nil, status.Error(codes.NotFound, "not_found")
	}

	return &listingv1.GetListingResponse{Listing: toWireFor(ctx, l)}, nil
}

// ListListings returns a keyset page of listings.
func (h *ListingHandler) ListListings(
	ctx context.Context,
	req *listingv1.ListListingsRequest,
) (*listingv1.ListListingsResponse, error) {
	if _, err := auth.RequireScopesAllowAnonymous(ctx, "listing.read"); err != nil {
		return nil, err
	}

	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}

	// An empty status means published. Any other status (draft, rejected) is a
	// moderation view: admin or internal service only. A seller's own drafts
	// live on ListMyListings.
	st := req.GetStatus()
	if st == "" {
		st = statusPublished
	}
	if st != statusPublished {
		_, admin := principalOwner(ctx)
		if !admin && !isService(ctx) {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
	}

	page, err := h.svc.List(ctx, cursor, pageSize, st)
	if err != nil {
		return nil, fail(ctx, err)
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
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
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
		return nil, fail(ctx, err)
	}
	mine := make([]*listingv1.Listing, 0, len(page.Items))
	for _, l := range page.Items {
		mine = append(mine, toWireFor(ctx, l))
	}
	return &listingv1.ListMyListingsResponse{
		Listings: mine,
		Page:     &commonv1.PageResponse{NextCursor: page.NextCursor, Total: page.Total},
	}, nil
}

// CreateListing stores a new listing (seller "đăng bán") and emits a
// ListingChanged(CREATED) event so read-models (team-search) can index it.
func (h *ListingHandler) CreateListing(
	ctx context.Context,
	req *listingv1.CreateListingRequest,
) (*listingv1.CreateListingResponse, error) {
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetListing() == nil {
		return nil, status.Error(codes.InvalidArgument, "listing is required")
	}
	ownerID, _ := principalOwner(ctx)
	created, err := h.svc.CreateWithEvent(ctx, fromWire(req.GetListing()), ownerID,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_CREATED))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.CreateListingResponse{Listing: toWire(created)}, nil
}

// UpdateListing replaces an existing listing and emits ListingChanged(UPDATED).
func (h *ListingHandler) UpdateListing(
	ctx context.Context,
	req *listingv1.UpdateListingRequest,
) (*listingv1.UpdateListingResponse, error) {
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetListing() == nil || req.GetListing().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing.id is required")
	}
	ownerID, isAdmin := principalOwner(ctx)
	l := fromWire(req.GetListing())
	updated, err := h.svc.UpdateWithEvent(ctx, l, ownerID, isAdmin,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.UpdateListingResponse{Listing: toWire(updated)}, nil
}

// DeleteListing removes the caller's own listing and emits ListingChanged(DELETED).
func (h *ListingHandler) DeleteListing(
	ctx context.Context,
	req *listingv1.DeleteListingRequest,
) (*listingv1.DeleteListingResponse, error) {
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ownerID, isAdmin := principalOwner(ctx)
	_, err := h.svc.DeleteWithEvent(ctx, req.GetId(), ownerID, isAdmin,
		h.enqueueListingChanged(ctx, listingv1.ChangeType_CHANGE_TYPE_DELETED))
	if err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.DeleteListingResponse{}, nil
}

// GetImageUploadUrl returns a presigned S3 PUT URL for uploading a product image.
func (h *ListingHandler) GetImageUploadUrl(
	ctx context.Context,
	req *listingv1.GetImageUploadUrlRequest,
) (*listingv1.GetImageUploadUrlResponse, error) {
	if _, err := auth.RequireScopes(ctx, "listing.write"); err != nil {
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
		return nil, fail(ctx, err)
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
	if _, err := auth.RequireScopesAllowAnonymous(ctx, "listing.read"); err != nil {
		return nil, err
	}
	if h.categories == nil {
		return &listingv1.ListCategoriesResponse{}, nil
	}
	items, err := h.categories.List(ctx, req.GetParentId())
	if err != nil {
		return nil, fail(ctx, err)
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
	if _, err := auth.RequireScopesAllowAnonymous(ctx, "listing.read"); err != nil {
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
		return nil, fail(ctx, err)
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
	// Internal inventory mutation: only the order service (service principal with
	// listing.write) may call it; end users are denied even if forwarded.
	if _, err := auth.RequireService(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be > 0")
	}
	// Idempotent on the caller-supplied reservation_id: a retried checkout with
	// the id of an active or committed reservation is a no-op, so stock is
	// decremented exactly once. There is no reserve without an id, so every
	// decrement leaves a reservation that can be committed, released and swept.
	if err := h.svc.ReserveStockIdempotent(ctx, req.GetReservationId(), req.GetListingId(), req.GetVariantId(), req.GetQuantity()); err != nil {
		if errors.Is(err, store.ErrOutOfStock) {
			return &listingv1.ReserveStockResponse{
				Success: false,
				Message: "insufficient stock",
			}, nil
		}
		if errors.Is(err, store.ErrReservationReleased) {
			return nil, status.Error(codes.FailedPrecondition, "reservation already released; reserve with a new reservation_id")
		}
		return nil, fail(ctx, err)
	}
	return &listingv1.ReserveStockResponse{Success: true}, nil
}

// ReleaseStock releases a reservation by reservation_id, restoring the quantity
// stored on it exactly once. listing_id, variant_id and quantity are ignored
// (kept on the wire for compatibility). A repeated, already swept or unknown id
// is a successful no-op; an empty id is INVALID_ARGUMENT.
func (h *ListingHandler) ReleaseStock(
	ctx context.Context,
	req *listingv1.ReleaseStockRequest,
) (*listingv1.ReleaseStockResponse, error) {
	// Internal inventory mutation: only the order service (service principal with
	// listing.write) may call it; end users are denied even if forwarded.
	if _, err := auth.RequireService(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if err := h.svc.ReleaseStock(ctx, req.GetReservationId()); err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.ReleaseStockResponse{Success: true}, nil
}

// CommitReservation makes an active reservation permanent (team-order calls it
// for every reservation of a checkout before placing the orders). Idempotent on
// reservation_id: OK for active or already committed; FAILED_PRECONDITION when
// the reservation was released (the order must not be placed); NOT_FOUND for an
// unknown id; INVALID_ARGUMENT for an empty id.
func (h *ListingHandler) CommitReservation(
	ctx context.Context,
	req *listingv1.CommitReservationRequest,
) (*listingv1.CommitReservationResponse, error) {
	// Internal inventory mutation: same authority as ReserveStock/ReleaseStock.
	if _, err := auth.RequireService(ctx, "listing.write"); err != nil {
		return nil, err
	}
	if req.GetReservationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if err := h.svc.CommitReservation(ctx, req.GetReservationId()); err != nil {
		return nil, fail(ctx, err)
	}
	return &listingv1.CommitReservationResponse{}, nil
}

// enqueueListingChanged returns the EnqueueFn the service invokes inside the
// write transaction. It captures the acting principal and request id from the
// context now (request-scoped), and — given the just-persisted listing — builds
// the ListingChanged EventEnvelope with a fresh event_id that becomes the outbox
// row's primary key (a stable dedupe id under at-least-once). No Kafka produce
// happens here: the row is written in the same tx as the listing, and the
// relayer publishes it later.
func (h *ListingHandler) enqueueListingChanged(ctx context.Context, change listingv1.ChangeType) store.EnqueueFn {
	principal := principalProto(ctx)
	requestID := reqctx.RequestID(ctx)
	return func(written store.Listing) (store.OutboxRow, error) {
		wire := toWire(written)
		wire.Moderation = moderationWire(written, false, false)
		eventID := uuid.NewString()
		payload, err := events.BuildListingChangedEnvelope(eventID, wire, change, principal, requestID)
		if err != nil {
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
			EventID:       eventID,
			AggregateType: "Listing",
			AggregateID:   written.ID,
			EventType:     events.ListingChangedEventType,
			Payload:       payload,
			RequestID:     requestID,
		}, nil
	}
}

// NewStockEventBuilder returns the store.StockEventBuilder that turns a
// stock snapshot into a ListingStockChanged outbox row. The repository calls it
// INSIDE the reserve / release / sweep transaction, so the row commits (or rolls
// back) with the stock change. The principal and request id come from the
// request context when present (the TTL sweep has neither).
func NewStockEventBuilder() store.StockEventBuilder {
	return func(ctx context.Context, snap store.StockSnapshot) (store.OutboxRow, error) {
		principal := principalProto(ctx)
		requestID := reqctx.RequestID(ctx)
		change := &listingv1.ListingStockChanged{
			ListingId: snap.ListingID,
			Stock:     snap.Stock,
			Variants:  toWire(store.Listing{Variants: snap.Variants}).GetVariants(),
		}
		eventID := uuid.NewString()
		payload, err := events.BuildListingStockChangedEnvelope(eventID, change, principal, requestID)
		if err != nil {
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
			EventID:       eventID,
			AggregateType: "Listing",
			AggregateID:   snap.ListingID,
			EventType:     events.ListingStockChangedEventType,
			Payload:       payload,
			RequestID:     requestID,
		}, nil
	}
}

const statusPublished = "published"

func principalProto(ctx context.Context) *commonv1.Principal {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return nil
	}
	t := commonv1.PrincipalType_PRINCIPAL_TYPE_UNSPECIFIED
	switch p.Type {
	case auth.TypeUser:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_USER
	case auth.TypeService:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE
	case auth.TypeAnonymous:
		t = commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS
	}
	return &commonv1.Principal{Id: p.ID, Type: t, Scopes: p.Scopes}
}

// isService reports whether the caller is an internal service principal
// (type SERVICE) holding listing.read. The type check matters: a user principal
// carrying the same scopes gets no extra access.
func isService(ctx context.Context) bool {
	p, ok := auth.FromContext(ctx)
	return ok && p.Type == auth.TypeService && p.HasScope("listing.read")
}

// canSeeNonPublished is the read rule for draft/rejected listings.
func canSeeNonPublished(ctx context.Context, sellerID string) bool {
	id, admin := principalOwner(ctx)
	if admin || isService(ctx) {
		return true
	}
	return id != "" && id == sellerID
}

// principalOwner extracts the owner id + admin flag from the context Principal.
func principalOwner(ctx context.Context) (id string, isAdmin bool) {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return "", false
	}
	return p.ID, p.HasScope("admin")
}

// fromWire maps a generated protobuf Listing to the domain value.
func fromWire(l *listingv1.Listing) store.Listing {
	variants := make([]store.Variant, 0, len(l.GetVariants()))
	for _, v := range l.GetVariants() {
		variants = append(variants, store.Variant{
			ID:        v.GetId(),
			ListingID: l.GetId(),
			Name:      v.GetName(),
			SKU:       v.GetSku(),
			Price:     v.GetPrice(),
			Stock:     v.GetStock(),
			ImageURL:  v.GetImageUrl(),
		})
	}
	return store.Listing{
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
	case listingv1.ListingStatus_LISTING_STATUS_BANNED:
		return "banned"
	case listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW:
		return "pending_review"
	case listingv1.ListingStatus_LISTING_STATUS_HELD:
		return "held"
	default:
		return ""
	}
}

// toWire maps a domain Listing to the generated protobuf message.
func toWire(l store.Listing) *listingv1.Listing {
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
func toWireList(items []store.Listing) []*listingv1.Listing {
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
	case "banned":
		return listingv1.ListingStatus_LISTING_STATUS_BANNED
	case "pending_review":
		return listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW
	case "held":
		return listingv1.ListingStatus_LISTING_STATUS_HELD
	default:
		return listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED
	}
}

// toWireFor maps a listing for a specific reader: the owner sees reason, note
// and decision time; admin and internal services also see screen flags;
// everyone else gets no moderation info.
func toWireFor(ctx context.Context, l store.Listing) *listingv1.Listing {
	w := toWire(l)
	id, admin := principalOwner(ctx)
	staff := admin || isService(ctx)
	owner := id != "" && id == l.SellerID
	if owner || staff {
		w.Moderation = moderationWire(l, true, staff)
	}
	return w
}

// moderationWire builds ModerationInfo; note and flags only when asked. It
// returns nil when the listing has no moderation data.
func moderationWire(l store.Listing, withNote, withFlags bool) *listingv1.ModerationInfo {
	if l.ModerationReasonCode == "" && l.ModerationNote == "" && len(l.ScreenFlags) == 0 && l.ModerationDecidedAt.IsZero() {
		return nil
	}
	m := &listingv1.ModerationInfo{ReasonCode: l.ModerationReasonCode}
	if withNote {
		m.Note = l.ModerationNote
	}
	if withFlags {
		m.ScreenFlags = l.ScreenFlags
	}
	if !l.ModerationDecidedAt.IsZero() {
		m.DecidedAt = timestamppb.New(l.ModerationDecidedAt)
	}
	return m
}
