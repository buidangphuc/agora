package handler

import (
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// internalErr logs the underlying error server-side and returns the generic
// INTERNAL status used across this handler, so driver/upstream error text never
// reaches the caller.
func internalErr(op string, err error) error {
	slog.Error("internal error", slog.String("op", op), slog.Any("error", err))
	return status.Error(codes.Internal, "internal error")
}
