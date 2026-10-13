package grpcx

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Rule is how one sentinel error is reported to the caller: the error to match
// with errors.Is, its gRPC code and, optionally, the client-facing message.
// An empty Message sends Target.Error(): the sentinel's own text, never the
// wrapped chain, so driver or upstream detail does not reach the caller.
type Rule struct {
	Target  error
	Code    codes.Code
	Message string
}

// ErrorTable is an ordered list of Rules; the first rule whose Target matches
// the error with errors.Is wins, so the order is the precedence. The zero
// value is an empty table. Build one with NewErrorTable.
type ErrorTable struct {
	rules []Rule
}

// NewErrorTable returns a table that tries rules in the given order.
func NewErrorTable(rules ...Rule) ErrorTable {
	return ErrorTable{rules: slices.Clone(rules)}
}

// Status converts err to a gRPC status error. An error that already is a gRPC
// status with a code other than Unknown (for example auth.RequirePrincipal's)
// is returned unchanged. context.Canceled and context.DeadlineExceeded map to
// codes.Canceled and codes.DeadlineExceeded and are logged at Info, not as
// errors: the caller went away or ran out of time. An error matching a table
// rule takes the first matching rule's code and its Message, or the Target's
// text when the Message is empty. Any other error is logged at error level and
// becomes codes.Internal with the message "internal error", so driver or
// upstream text never reaches the caller. A nil err returns nil.
func Status(ctx context.Context, logger *slog.Logger, err error, t ErrorTable) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return err
	}
	if logger == nil {
		logger = slog.Default()
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		logger.InfoContext(ctx, "request deadline exceeded", "err", err)
		return status.Error(codes.DeadlineExceeded, context.DeadlineExceeded.Error())
	case errors.Is(err, context.Canceled):
		logger.InfoContext(ctx, "request canceled", "err", err)
		return status.Error(codes.Canceled, context.Canceled.Error())
	}
	for _, rule := range t.rules {
		if errors.Is(err, rule.Target) {
			msg := rule.Message
			if msg == "" {
				msg = rule.Target.Error()
			}
			return status.Error(rule.Code, msg)
		}
	}
	logger.ErrorContext(ctx, "unhandled error", "err", err)
	return status.Error(codes.Internal, internalMessage)
}
