package rpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

// Error rules: a domain sentinel, its gRPC code and the fixed, caller-safe message
// clients already receive. A handler passes the rules that apply to one call to
// fail, so an error that is not listed there still becomes the generic INTERNAL
// answer (a sentinel that one RPC maps is not mapped by another).
var (
	ruleOrderNotFound         = grpcx.Rule{Target: store.ErrOrderNotFound, Code: codes.NotFound, Message: "order not found"}
	ruleReturnNotFound        = grpcx.Rule{Target: store.ErrReturnNotFound, Code: codes.NotFound, Message: "return request not found"}
	ruleShipmentNotFound      = grpcx.Rule{Target: store.ErrShipmentNotFound, Code: codes.NotFound, Message: "shipment not found"}
	ruleReorderNotOwner       = grpcx.Rule{Target: order.ErrReorderNotOwner, Code: codes.PermissionDenied, Message: "cannot reorder another user's order"}
	ruleEmptyCart             = grpcx.Rule{Target: order.ErrEmptyCart, Code: codes.FailedPrecondition, Message: "cart is empty"}
	ruleSelfPurchase          = grpcx.Rule{Target: order.ErrSelfPurchase, Code: codes.FailedPrecondition, Message: "cannot buy your own listing"}
	ruleInsufficientStock     = grpcx.Rule{Target: order.ErrInsufficientStock, Code: codes.ResourceExhausted, Message: "stock reservation failed: insufficient stock"}
	ruleCheckoutInProgress    = grpcx.Rule{Target: order.ErrCheckoutInProgress, Code: codes.Aborted, Message: "checkout with this idempotency key is in progress; retry"}
	ruleReservationLost       = grpcx.Rule{Target: order.ErrReservationLost, Code: codes.FailedPrecondition, Message: "item no longer reserved; please retry checkout"}
	ruleActorForbidden        = grpcx.Rule{Target: order.ErrActorForbidden, Code: codes.PermissionDenied, Message: "this status change is not allowed for the seller"}
	ruleUnauthorizedReturn    = grpcx.Rule{Target: order.ErrUnauthorizedReturn, Code: codes.PermissionDenied, Message: "only the buyer can request a return for this order"}
	ruleInvalidReturnReason   = grpcx.Rule{Target: order.ErrInvalidReturnReason, Code: codes.InvalidArgument, Message: "return reason is required"}
	ruleInvalidRefundAmount   = grpcx.Rule{Target: order.ErrInvalidRefundAmount, Code: codes.InvalidArgument, Message: "invalid refund amount"}
	ruleOrderCannotBeReturned = grpcx.Rule{Target: order.ErrOrderCannotBeReturned, Code: codes.FailedPrecondition, Message: "order cannot be returned in its current status"}
	ruleNothingToReturn       = grpcx.Rule{Target: order.ErrNothingToReturn, Code: codes.FailedPrecondition, Message: "order has no returnable amount left"}
	ruleInvalidReturnStatus   = grpcx.Rule{Target: order.ErrInvalidReturnStatus, Code: codes.FailedPrecondition, Message: "invalid return status transition"}
	ruleNotPaidOnline         = grpcx.Rule{Target: order.ErrNotPaidOnline, Code: codes.FailedPrecondition, Message: order.ErrNotPaidOnline.Error()}
)

// ruleInvalidStatus maps order.ErrInvalidStatus with the message of the RPC at
// hand ("cannot be cancelled", "cannot be shipped", ...).
func ruleInvalidStatus(msg string) grpcx.Rule {
	return grpcx.Rule{Target: order.ErrInvalidStatus, Code: codes.FailedPrecondition, Message: msg}
}

// fail converts err for the RPC op. An error that matches one of rules gets that
// rule's code and fixed message through grpcx.Status; anything else is logged
// server-side and becomes INTERNAL "<op> failed", so driver or upstream error
// text never reaches the caller.
func fail(ctx context.Context, logger *slog.Logger, op string, err error, rules ...grpcx.Rule) error {
	for _, r := range rules {
		if !errors.Is(err, r.Target) {
			continue
		}
		table, terr := grpcx.NewErrorTable(rules...)
		if terr != nil {
			return internalErr(logger, op, terr)
		}
		return grpcx.Status(ctx, logger, err, table)
	}
	return internalErr(logger, op, err)
}

// internalErr logs the underlying error server-side and returns a generic
// INTERNAL status naming only the operation. grpcx.Status answers "internal error"
// for an unlisted error, which would change this text for clients, so the
// per-operation message stays here.
func internalErr(logger *slog.Logger, op string, err error) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("internal error", "op", op, "error", err)
	return status.Error(codes.Internal, op+" failed")
}
