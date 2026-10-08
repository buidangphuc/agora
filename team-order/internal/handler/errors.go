package handler

import (
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// internalErr logs the underlying error server-side and returns a generic
// INTERNAL status, so driver/upstream error text never reaches the caller.
func internalErr(logger *slog.Logger, op string, err error) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("internal error", slog.String("op", op), slog.Any("error", err))
	return status.Error(codes.Internal, op+" failed")
}

// clientErr logs the detailed cause server-side and returns a stable,
// caller-safe message with the given (non-INTERNAL) code.
func clientErr(logger *slog.Logger, code codes.Code, msg string, err error) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("request rejected", slog.String("client_msg", msg), slog.Any("error", err))
	return status.Error(code, msg)
}
