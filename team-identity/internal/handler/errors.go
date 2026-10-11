package handler

import (
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// internalErr logs the underlying error server-side and returns a generic
// INTERNAL status, so driver error text never reaches the caller.
func internalErr(logger *slog.Logger, op string, err error) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("internal error", slog.String("op", op), slog.Any("error", err))
	return status.Error(codes.Internal, "internal error")
}
