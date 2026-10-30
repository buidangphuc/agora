package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/team-analytics/internal/query"
)

func newErrorTable() (grpcx.ErrorTable, error) {
	return grpcx.NewErrorTable(
		grpcx.Rule{Target: query.ErrUnsupported, Code: codes.Unavailable, Message: "not available on this warehouse"},
	)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}
