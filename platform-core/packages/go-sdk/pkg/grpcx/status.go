package grpcx

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Rule is how one sentinel error is reported to the caller: its gRPC code and,
// optionally, the client-facing message. An empty Message keeps err.Error().
type Rule struct {
	Code    codes.Code
	Message string
}

// Code returns a Rule with code c that keeps the error's own text as message.
func Code(c codes.Code) Rule { return Rule{Code: c} }

// ErrorTable maps a service's sentinel errors to Rules. Entries are matched
// with errors.Is; if several entries match one error, which one wins is
// unspecified, so keep sentinels distinct.
type ErrorTable map[error]Rule

// Status converts err to a gRPC status error. An error that already is a gRPC
// status with a code other than Unknown (for example auth.RequirePrincipal's)
// is returned unchanged. An error matching a table entry takes the rule's
// code and its Message, or err.Error() when the Message is empty. Any other error is logged and becomes
// codes.Internal with the message "internal error", so driver or upstream text
// never reaches the caller. A nil err returns nil.
func Status(ctx context.Context, logger *slog.Logger, err error, t ErrorTable) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return err
	}
	for target, rule := range t {
		if errors.Is(err, target) {
			msg := rule.Message
			if msg == "" {
				msg = err.Error()
			}
			return status.Error(rule.Code, msg)
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(ctx, "unhandled error", "err", err)
	return status.Error(codes.Internal, internalMessage)
}
