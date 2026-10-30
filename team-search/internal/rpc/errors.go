package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/team-search/internal/store"
)

func newErrorTable() (grpcx.ErrorTable, error) {
	return grpcx.NewErrorTable(
		grpcx.Rule{Target: store.ErrSavedSearchNotFound, Code: codes.NotFound, Message: "saved search not found"},
	)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}
