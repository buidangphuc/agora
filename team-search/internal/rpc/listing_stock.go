package rpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
)

const (
	maxListingStockIDs   = 10
	listingStockDeadline = 200 * time.Millisecond
)

func (h *Handler) GetListingStock(ctx context.Context, req *searchv1.GetListingStockRequest) (*searchv1.GetListingStockResponse, error) {
	ids := req.GetListingIds()
	if len(ids) < 1 || len(ids) > maxListingStockIDs {
		return nil, status.Errorf(codes.InvalidArgument, "listing_ids must hold 1..%d ids", maxListingStockIDs)
	}
	for _, id := range ids {
		if id == "" {
			return nil, status.Error(codes.InvalidArgument, "listing_ids must not hold an empty id")
		}
	}
	if _, err := auth.RequireService(ctx, scopeSearchRead); err != nil {
		return nil, err
	}
	reader, ok := h.idx.(index.ListingStockReader)
	if !ok {
		return nil, status.Error(codes.Unavailable, "listing stock lookup unavailable")
	}
	lctx, cancel := context.WithTimeout(ctx, listingStockDeadline)
	defer cancel()
	stocks, err := reader.ListingStocks(lctx, ids)
	if err != nil {
		h.logger.WarnContext(ctx, "listing stock lookup failed", "method", "GetListingStock", "id_count", len(ids), "error", err)
		return nil, status.Error(codes.Unavailable, "listing stock lookup unavailable")
	}
	entries := make([]*searchv1.ListingStock, 0, len(ids))
	for _, id := range ids {
		e := &searchv1.ListingStock{ListingId: id}
		if v, ok := stocks[id]; ok {
			e.Stock = &v
		}
		entries = append(entries, e)
	}
	h.logger.InfoContext(ctx, "listing stock lookup", "method", "GetListingStock", "id_count", len(ids))
	return &searchv1.GetListingStockResponse{Listings: entries}, nil
}
