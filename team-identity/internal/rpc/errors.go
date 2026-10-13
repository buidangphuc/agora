package rpc

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

// authErrors maps the auth domain's sentinel errors to the code and the text the
// client has always seen. The first matching rule wins; anything else becomes
// INTERNAL "internal error" (logged, never sent).
var authErrors = mustErrorTable(
	grpcx.Rule{Target: identity.ErrInvalidInput, Code: codes.InvalidArgument, Message: "invalid input"},
	grpcx.Rule{Target: identity.ErrInvalidCredentials, Code: codes.Unauthenticated, Message: "invalid credentials"},
	grpcx.Rule{Target: identity.ErrInvalidToken, Code: codes.InvalidArgument, Message: "invalid reset token"},
	grpcx.Rule{Target: identity.ErrTokenExpired, Code: codes.InvalidArgument, Message: "reset token expired"},
	grpcx.Rule{Target: identity.ErrTokenAlreadyUsed, Code: codes.InvalidArgument, Message: "reset token already used"},
	grpcx.Rule{Target: store.ErrConflict, Code: codes.AlreadyExists, Message: "username already exists"},
	grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound, Message: "user not found"},
	grpcx.Rule{Target: store.ErrTokenNotFound, Code: codes.NotFound, Message: "reset token not found"},
)

// mustErrorTable builds a table from rules fixed in source; a bad rule is a
// programming error that fails at start-up.
func mustErrorTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}

// internalErr logs the underlying error server-side, with the operation that
// failed, and returns a generic INTERNAL status, so driver error text never
// reaches the caller.
func internalErr(ctx context.Context, logger *slog.Logger, op string, err error) error {
	return grpcx.Status(ctx, logger, fmt.Errorf("%s: %w", op, err), grpcx.ErrorTable{})
}
