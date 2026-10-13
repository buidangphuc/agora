package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	"github.com/buidangphuc/team-promotion/internal/promotion"
	"github.com/buidangphuc/team-promotion/internal/store"
)

// errorTable maps the domain sentinels to the status the client has always
// seen. An empty Message sends the sentinel's own text; the two sentinels whose
// text differs from the client text carry it explicitly.
var errorTable = mustErrorTable(
	grpcx.Rule{Target: promotion.ErrInvalidVoucher, Code: codes.InvalidArgument},
	grpcx.Rule{Target: promotion.ErrInvalidCampaign, Code: codes.InvalidArgument, Message: "invalid campaign"},
	grpcx.Rule{Target: promotion.ErrInvalidAdCampaign, Code: codes.InvalidArgument},
	grpcx.Rule{Target: promotion.ErrInvalidSubscription, Code: codes.InvalidArgument},
	grpcx.Rule{Target: promotion.ErrPlanNotFound, Code: codes.NotFound},
	grpcx.Rule{Target: store.ErrVoucherNotFound, Code: codes.NotFound},
	grpcx.Rule{Target: store.ErrCampaignNotFound, Code: codes.NotFound, Message: "campaign not found"},
)

func mustErrorTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}

// internalError turns a handler error into a gRPC status: a domain sentinel in
// errorTable gets its mapped code and text, an error that already is a status
// passes through, and anything else is logged with the operation that failed and
// returned as a generic INTERNAL, so storage or upstream details never reach the
// client. err goes in unwrapped: a status error keeps its own message.
func internalError(ctx context.Context, logger *slog.Logger, op string, err error) error {
	return grpcx.Status(ctx, logger.With("op", op), err, errorTable)
}
