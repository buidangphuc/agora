package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-notification/internal/store"
)

// mustTable builds an error table from static rules; a bad rule is a
// programming error, so it fails at start-up like regexp.MustCompile.
func mustTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}

// notificationErrs maps the inbox repository's sentinel errors; the message is
// the sentinel's own text.
var notificationErrs = mustTable(
	grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound},
)

// internalErr logs the cause server-side and returns a generic INTERNAL
// status so driver / database error text never reaches callers.
func internalErr(ctx context.Context, op string, err error) error {
	return grpcx.Status(ctx, slog.With("op", op), err, grpcx.ErrorTable{})
}
