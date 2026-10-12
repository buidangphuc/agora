package rpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
)

// internalError logs the real cause (with the operation that failed) and returns
// a generic INTERNAL, so storage or upstream details never reach the client. An
// error that already is a gRPC status passes through unchanged. The handlers map
// their own domain errors to codes before they get here, because the messages
// they return differ from the sentinels' text.
func internalError(ctx context.Context, logger *slog.Logger, op string, err error) error {
	return grpcx.Status(ctx, logger, fmt.Errorf("%s: %w", op, err), nil)
}
