package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
)

const (
	msgNotFound      = "not_found"
	msgStatusChange  = "listing status does not allow this change"
	msgStatusDecide  = "listing status does not allow this decision"
	msgNotReservable = "listing is not reservable"
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
		grpcx.Rule{Target: store.ErrInvalidCursor, Code: codes.InvalidArgument, Message: "invalid cursor"},
		grpcx.Rule{Target: listing.ErrSlugRequired, Code: codes.InvalidArgument, Message: "slug is required"},
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
	return grpcx.Status(ctx, nil, err, errorTable)
}
