package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
)

// TEST-STUB: compile-only placeholder written by the test author. The implementer
// replaces this file with the real handler (git mv of internal/handler/*.go).

type OrderPartiesReader interface {
	GetOrderParties(ctx context.Context, orderID string) (buyerID, sellerID string, err error)
}

type ReturnReader interface {
	GetReturn(ctx context.Context, returnID string) (orderID string, status orderv1.ReturnStatus, canEscalate bool, err error)
}

type Handler struct {
	engagementv1.UnimplementedEngagementServiceServer
}

func NewHandler(
	_ store.Repository,
	_ *engagement.ReviewService,
	_ *engagement.QAService,
	_ *engagement.DisputeService,
	_ *engagement.CollectionService,
	_ *engagement.ReportService,
	_ OrderPartiesReader,
	_ ReturnReader,
	_ *slog.Logger,
) (*Handler, error) {
	return &Handler{}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	engagementv1.RegisterEngagementServiceServer(srv, h)
}
