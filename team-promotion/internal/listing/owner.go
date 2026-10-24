package listing

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

var (
	ErrNotFound     = errors.New("listing not found")
	ErrNotOwner     = errors.New("listing belongs to another seller")
	ErrLookupFailed = errors.New("listing lookup failed")
)

type Getter interface {
	GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
}

func Owner(ctx context.Context, listings Getter, logger *slog.Logger, listingID, callerID string) error {
	resp, err := listings.GetListing(ctx, &listingv1.GetListingRequest{Id: listingID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		logger.Warn("listing ownership lookup failed", slog.String("listing_id", listingID), slog.Any("err", err))
		return ErrLookupFailed
	}
	owner := resp.GetListing().GetSellerId()
	if owner == "" || callerID == "" || owner != callerID {
		return ErrNotOwner
	}
	return nil
}
