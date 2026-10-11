package handler

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// internalErr logs the cause server-side and returns a generic INTERNAL
// status so driver / database error text never reaches callers.
func internalErr(ctx context.Context, op string, err error) error {
	slog.ErrorContext(ctx, "internal error", "op", op, "error", err)
	return status.Error(codes.Internal, "internal error")
}
