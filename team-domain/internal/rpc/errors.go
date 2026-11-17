package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
)

const (
	msgNotFound      = "not_found"
	msgStatusChange  = "listing status does not allow this change"
	msgStatusDecide  = "listing status does not allow this decision"
	msgNotReservable = "listing is not reservable"
	msgListingCap    = "listing limit of the current plan reached"
	msgEntitlements  = "plan entitlements unavailable"
	reasonListingCap = "PLAN_LISTING_LIMIT"
	msgFeaturedCap   = "featured listings limit of the current plan exceeded"
	reasonFeatured   = "PLAN_FEATURED_LIMIT"
	errorDomain      = "team-domain"
	msgReusedID      = "reservation_id already used for a different listing, variant or quantity"
)

func newErrorTable() grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound, Message: msgNotFound},
		grpcx.Rule{Target: store.ErrVariantNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: store.ErrReservationNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: store.ErrCategoryNotFound, Code: codes.NotFound},
		grpcx.Rule{Target: store.ErrBundleNotFound, Code: codes.NotFound, Message: msgNotFound},
		grpcx.Rule{Target: store.ErrStorefrontNotFound, Code: codes.NotFound, Message: msgNotFound},
		grpcx.Rule{Target: store.ErrSlugTaken, Code: codes.AlreadyExists, Message: "slug already taken"},
		grpcx.Rule{Target: listing.ErrStatusLocked, Code: codes.FailedPrecondition, Message: msgStatusChange},
		grpcx.Rule{Target: store.ErrStatusChanged, Code: codes.FailedPrecondition, Message: msgStatusChange},
		grpcx.Rule{Target: store.ErrReservationReleased, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: store.ErrListingNotReservable, Code: codes.FailedPrecondition, Message: msgNotReservable},
		grpcx.Rule{Target: store.ErrReservationMismatch, Code: codes.FailedPrecondition, Message: msgReusedID},
		grpcx.Rule{Target: listing.ErrTransitionNotAllowed, Code: codes.FailedPrecondition, Message: msgStatusDecide},
		grpcx.Rule{Target: listing.ErrForbidden, Code: codes.PermissionDenied},
		grpcx.Rule{Target: listing.ErrBundleListingNotOwned, Code: codes.PermissionDenied, Message: "listing not found or not yours"},
		grpcx.Rule{Target: listing.ErrBundleListingCount, Code: codes.InvalidArgument, Message: "a bundle needs 2 to 10 listings"},
		grpcx.Rule{Target: listing.ErrBundleDuplicateListing, Code: codes.InvalidArgument, Message: "a bundle cannot repeat a listing"},
		grpcx.Rule{Target: listing.ErrBundleVariantListing, Code: codes.InvalidArgument, Message: "a listing with variants cannot be bundled"},
		grpcx.Rule{Target: listing.ErrBundlePrice, Code: codes.InvalidArgument, Message: "bundle price must be above 0 and below the sum of the listings"},
		grpcx.Rule{Target: store.ErrInvalidCursor, Code: codes.InvalidArgument, Message: "invalid cursor"},
		grpcx.Rule{Target: listing.ErrSlugRequired, Code: codes.InvalidArgument, Message: "slug is required"},
		grpcx.Rule{Target: listing.ErrTaglineInvalid, Code: codes.InvalidArgument, Message: "invalid tagline"},
		grpcx.Rule{Target: listing.ErrBannerInvalid, Code: codes.InvalidArgument, Message: "invalid banner"},
		grpcx.Rule{Target: listing.ErrThemeInvalid, Code: codes.InvalidArgument, Message: "invalid theme"},
		grpcx.Rule{Target: listing.ErrFeaturedInvalid, Code: codes.InvalidArgument, Message: "invalid featured listings"},
		grpcx.Rule{Target: listing.ErrTooManyFeatured, Code: codes.InvalidArgument, Message: "too many featured listings (max 6)"},
		grpcx.Rule{Target: listing.ErrDisplayNameInvalid, Code: codes.InvalidArgument, Message: "invalid display_name"},
		grpcx.Rule{Target: listing.ErrTitleRequired, Code: codes.InvalidArgument, Message: "title is required"},
		grpcx.Rule{Target: listing.ErrTooManySellerIDs, Code: codes.InvalidArgument, Message: "too many seller_ids (max 100)"},
	)
	if err != nil {
		panic(err)
	}
	return t
}

var errorTable = newErrorTable()

func fail(ctx context.Context, err error) error {
	if errors.Is(err, store.ErrListingCapReached) {
		slog.InfoContext(ctx, "listing create rejected at plan cap")
		st := status.New(codes.ResourceExhausted, msgListingCap)
		if withInfo, derr := st.WithDetails(&errdetails.ErrorInfo{Reason: reasonListingCap, Domain: errorDomain}); derr == nil {
			st = withInfo
		}
		return st.Err()
	}
	if errors.Is(err, listing.ErrFeaturedLimit) {
		st := status.New(codes.ResourceExhausted, msgFeaturedCap)
		if withInfo, derr := st.WithDetails(&errdetails.ErrorInfo{Reason: reasonFeatured, Domain: errorDomain}); derr == nil {
			st = withInfo
		}
		return st.Err()
	}
	if errors.Is(err, listing.ErrEntitlementsUnavailable) {
		slog.WarnContext(ctx, "plan entitlements unavailable", "err", err)
		return status.Error(codes.Unavailable, msgEntitlements)
	}
	return grpcx.Status(ctx, nil, err, errorTable)
}
