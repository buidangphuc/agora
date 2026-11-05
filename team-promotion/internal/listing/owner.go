package listing

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
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

func Get(ctx context.Context, listings Getter, logger *slog.Logger, listingID string) (*listingv1.Listing, error) {
	resp, err := listings.GetListing(ctx, &listingv1.GetListingRequest{Id: listingID})
	if err != nil {
		if status.Convert(err).Proto().GetCode() == int32(code.Code_NOT_FOUND) {
			return nil, ErrNotFound
		}
		logger.WarnContext(ctx, "listing ownership lookup failed", "listing_id", listingID, "err", err)
		return nil, ErrLookupFailed
	}
	return resp.GetListing(), nil
}

func CheckOwner(l *listingv1.Listing, callerID string) error {
	owner := l.GetSellerId()
	if owner == "" || callerID == "" || owner != callerID {
		return ErrNotOwner
	}
	return nil
}

func Owner(ctx context.Context, listings Getter, logger *slog.Logger, listingID, callerID string) error {
	l, err := Get(ctx, listings, logger, listingID)
	if err != nil {
		return err
	}
	return CheckOwner(l, callerID)
}
