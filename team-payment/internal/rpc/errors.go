package rpc

import (
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
)

// Each RPC family has its own ordered ErrorTable, because the same sentinel is worded
// differently per RPC (ErrInvalidAmount). A Rule with no Message sends the sentinel's own
// text. Anything not listed, and every error that is already a gRPC status, goes through
// grpcx.Status: unknown errors become INTERNAL "internal error". *store.FundsOnHoldError
// stays out of the tables: its client text carries the release instant (held.Error()).
var (
	createErrs = mustTable(
		grpcx.Rule{Target: payment.ErrOrderNotFound, Code: codes.NotFound, Message: "order not found"},
		grpcx.Rule{Target: payment.ErrNotOrderBuyer, Code: codes.PermissionDenied, Message: "only the order's buyer can pay for it"},
		grpcx.Rule{Target: payment.ErrInvalidOrderState, Code: codes.FailedPrecondition, Message: "order is not in pending state"},
	)
	getErrs  = mustTable(transactionNotFound)
	mockErrs = mustTable(
		transactionNotFound,
		grpcx.Rule{Target: payment.ErrPaymentRefunded, Code: codes.FailedPrecondition},
	)
	// sellerLookupErrs maps the team-order seller lookup (GetPayment reader gate, refund gate).
	sellerLookupErrs = mustTable(
		grpcx.Rule{Target: payment.ErrOrderServiceUnavailable, Code: codes.Unavailable, Message: "order service unavailable"},
		grpcx.Rule{Target: payment.ErrOrderNotFound, Code: codes.NotFound, Message: "order not found"},
	)
	payoutErrs = mustTable(
		grpcx.Rule{Target: store.ErrInsufficientBalance, Code: codes.FailedPrecondition, Message: "insufficient wallet balance"},
		grpcx.Rule{Target: store.ErrInvalidAmount, Code: codes.InvalidArgument, Message: "invalid payout amount"},
	)
	walletPayoutErrs = mustTable(
		grpcx.Rule{Target: store.ErrInvalidAmount, Code: codes.InvalidArgument},
		grpcx.Rule{Target: store.ErrInsufficientBalance, Code: codes.FailedPrecondition, Message: "insufficient wallet balance"},
	)
	ledgerErrs = mustTable(
		grpcx.Rule{Target: payment.ErrInvalidPageToken, Code: codes.InvalidArgument, Message: "invalid page cursor"},
	)
	refundLookupErrs = mustTable(transactionNotFound)
	refundErrs       = mustTable(
		transactionNotFound,
		grpcx.Rule{Target: payment.ErrExceedsRemainder, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrInvalidRefund, Code: codes.FailedPrecondition},
		grpcx.Rule{Target: payment.ErrRefundIDConflict, Code: codes.AlreadyExists, Message: "refund_id was already used for another payment or amount"},
		grpcx.Rule{Target: payment.ErrInvalidRefundID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: payment.ErrInvalidAmount, Code: codes.InvalidArgument},
	)
	transactionNotFound = grpcx.Rule{Target: store.ErrTransactionNotFound, Code: codes.NotFound, Message: "transaction not found"}
)

// mustTable builds a table of static rules; a bad rule is a programming error.
func mustTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}
