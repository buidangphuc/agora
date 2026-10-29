package rpc

import (
	"errors"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"google.golang.org/grpc"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
)

type OrderHandler struct {
	orderv1.UnimplementedOrderServiceServer
}

type CartHandler struct {
	orderv1.UnimplementedCartServiceServer
}

func NewOrderHandler(_ *order.OrderService, _ identityv1.AddressServiceClient, _ flags.Evaluator, _ *auditx.Gate, _ *slog.Logger) (*OrderHandler, error) {
	return nil, errors.New("TEST-STUB: NewOrderHandler not implemented")
}

func NewCartHandler(_ *order.CartService, _ *slog.Logger) *CartHandler {
	return &CartHandler{}
}

func Register(_ *grpc.Server, _ *CartHandler, _ *OrderHandler) {
	panic("TEST-STUB: Register not implemented")
}
