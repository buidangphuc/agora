package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
)

func (h *EngagementHandler) CreateCollection(
	ctx context.Context, req *engagementv1.CreateCollectionRequest,
) (*engagementv1.CreateCollectionResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if h.collectionSvc == nil {
		return nil, status.Error(codes.Unimplemented, "collections service unavailable")
	}

	col, err := h.collectionSvc.CreateCollection(ctx, userID(ctx), req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create collection: %v", err)
	}
	return &engagementv1.CreateCollectionResponse{Collection: toWireCollection(col)}, nil
}

func (h *EngagementHandler) ListCollections(
	ctx context.Context, _ *engagementv1.ListCollectionsRequest,
) (*engagementv1.ListCollectionsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if h.collectionSvc == nil {
		return &engagementv1.ListCollectionsResponse{}, nil
	}

	cols, err := h.collectionSvc.ListCollections(ctx, userID(ctx))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list collections: %v", err)
	}
	wire := make([]*engagementv1.Collection, 0, len(cols))
	for _, c := range cols {
		wire = append(wire, toWireCollection(c))
	}
	return &engagementv1.ListCollectionsResponse{Collections: wire}, nil
}

func (h *EngagementHandler) AddToCollection(
	ctx context.Context, req *engagementv1.AddToCollectionRequest,
) (*engagementv1.AddToCollectionResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetCollectionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "collection_id is required")
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if h.collectionSvc == nil {
		return nil, status.Error(codes.Unimplemented, "collections service unavailable")
	}

	if _, err := h.collectionSvc.AddToCollection(ctx, userID(ctx), req.GetCollectionId(), req.GetListingId()); err != nil {
		if errors.Is(err, repository.ErrCollectionNotFound) {
			return nil, status.Error(codes.NotFound, "collection not found")
		}
		return nil, status.Errorf(codes.Internal, "add to collection: %v", err)
	}
	return &engagementv1.AddToCollectionResponse{}, nil
}

func (h *EngagementHandler) RemoveFromCollection(
	ctx context.Context, req *engagementv1.RemoveFromCollectionRequest,
) (*engagementv1.RemoveFromCollectionResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetCollectionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "collection_id is required")
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if h.collectionSvc == nil {
		return nil, status.Error(codes.Unimplemented, "collections service unavailable")
	}

	if _, err := h.collectionSvc.RemoveFromCollection(ctx, userID(ctx), req.GetCollectionId(), req.GetListingId()); err != nil {
		if errors.Is(err, repository.ErrCollectionNotFound) {
			return nil, status.Error(codes.NotFound, "collection not found")
		}
		return nil, status.Errorf(codes.Internal, "remove from collection: %v", err)
	}
	return &engagementv1.RemoveFromCollectionResponse{}, nil
}

func (h *EngagementHandler) ListCollectionItems(
	ctx context.Context, req *engagementv1.ListCollectionItemsRequest,
) (*engagementv1.ListCollectionItemsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetCollectionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "collection_id is required")
	}
	if h.collectionSvc == nil {
		return &engagementv1.ListCollectionItemsResponse{}, nil
	}

	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}

	ids, next, total, err := h.collectionSvc.ListCollectionItems(ctx, userID(ctx), req.GetCollectionId(), cursor, pageSize)
	if err != nil {
		if errors.Is(err, repository.ErrCollectionNotFound) {
			return nil, status.Error(codes.NotFound, "collection not found")
		}
		return nil, status.Errorf(codes.Internal, "list collection items: %v", err)
	}
	return &engagementv1.ListCollectionItemsResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func toWireCollection(c repository.Collection) *engagementv1.Collection {
	return &engagementv1.Collection{
		Id:        c.ID,
		UserId:    c.UserID,
		Name:      c.Name,
		ItemCount: c.ItemCount,
		CreatedAt: timestamppb.New(c.CreatedAt),
	}
}
