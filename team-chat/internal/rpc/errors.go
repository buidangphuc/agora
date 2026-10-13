package rpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
)

// internalErr logs the cause server-side and returns a generic INTERNAL
// status so driver / database error text never reaches callers.
func internalErr(ctx context.Context, logger *slog.Logger, op string, err error) error {
	return grpcx.Status(ctx, logger, fmt.Errorf("%s: %w", op, err), grpcx.ErrorTable{})
}
