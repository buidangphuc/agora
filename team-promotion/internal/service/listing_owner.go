package service

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// requireListingOwner looks the listing up in team-domain as the caller and
// requires its seller_id to equal callerID. Fails closed when no client is wired.
// Upstream failures (including timeouts) are a generic UNAVAILABLE; the cause is
// logged, never returned.
func requireListingOwner(ctx context.Context, listings upstream.ListingGetter, logger *slog.Logger, listingID, callerID string) error {
	if listings == nil {
		return status.Error(codes.Unavailable, "listing ownership cannot be verified (UPSTREAM_DOMAIN_ADDR not configured)")
	}
	resp, err := listings.GetListing(ctx, &listingv1.GetListingRequest{Id: listingID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return status.Error(codes.InvalidArgument, "listing not found")
		}
		logger.Warn("listing ownership lookup failed", slog.String("listing_id", listingID), slog.Any("err", err))
		return status.Error(codes.Unavailable, "listing lookup failed")
	}
	owner := resp.GetListing().GetSellerId()
	if owner == "" || callerID == "" || owner != callerID {
		return status.Error(codes.PermissionDenied, "listing belongs to another seller")
	}
	return nil
}
