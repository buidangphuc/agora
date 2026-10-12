package grpcx

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorTable maps a service's sentinel errors to gRPC codes. Entries are
// matched with errors.Is; if several entries match one error, which one wins
// is unspecified, so keep sentinels distinct.
type ErrorTable map[error]codes.Code

// Status converts err to a gRPC status error. An error that already is a gRPC
// status with a code other than Unknown (for example auth.RequirePrincipal's)
// is returned unchanged. An error matching a table entry
// keeps its code and its message. Any other error is logged and becomes
// codes.Internal with the message "internal error", so driver or upstream text
// never reaches the caller. A nil err returns nil.
func Status(ctx context.Context, logger *slog.Logger, err error, t ErrorTable) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return err
	}
	for target, code := range t {
		if errors.Is(err, target) {
			return status.Error(code, err.Error())
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(ctx, "unhandled error", "err", err)
	return status.Error(codes.Internal, internalMessage)
}
