package rpc

import (
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
)

// internalErr logs the underlying error server-side and returns the generic
// INTERNAL status used across this handler, so driver/upstream error text never
// reaches the caller.
func internalErr(op string, err error) error {
	slog.Error("internal error", "op", op, "error", err)
	return status.Error(codes.Internal, "internal error")
}

// The error tables the handlers pass to grpcx.Status. Each Message is the fixed
// string clients already see; any error not listed becomes Internal with
// "internal error".
var (
	// listingErrs maps the listing write/read paths.
	listingErrs = mustErrorTable(
		grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound, Message: "not_found"},
		grpcx.Rule{Target: listing.ErrForbidden, Code: codes.PermissionDenied, Message: "not the listing owner"},
	)
	// sellerErrs maps the bundle and storefront paths.
	sellerErrs = mustErrorTable(
		grpcx.Rule{Target: listing.ErrTitleRequired, Code: codes.InvalidArgument, Message: "title is required"},
		grpcx.Rule{Target: listing.ErrSlugRequired, Code: codes.InvalidArgument, Message: "slug is required"},
		grpcx.Rule{Target: listing.ErrDisplayNameInvalid, Code: codes.InvalidArgument, Message: "invalid display_name"},
		grpcx.Rule{Target: listing.ErrTooManySellerIDs, Code: codes.InvalidArgument, Message: "too many seller_ids (max 100)"},
		grpcx.Rule{Target: listing.ErrForbidden, Code: codes.PermissionDenied, Message: "no authenticated seller"},
		grpcx.Rule{Target: store.ErrSlugTaken, Code: codes.AlreadyExists, Message: "slug already taken"},
		grpcx.Rule{Target: store.ErrBundleNotFound, Code: codes.NotFound, Message: "not_found"},
		grpcx.Rule{Target: store.ErrStorefrontNotFound, Code: codes.NotFound, Message: "not_found"},
	)
)

// mustErrorTable builds a table at package init. A bad rule is a programming
// error, so it panics before the service takes traffic.
func mustErrorTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}
