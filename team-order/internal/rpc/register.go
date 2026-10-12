package rpc

import (
	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
)

// Register mounts the cart and order services on srv. A nil handler is skipped.
func Register(srv *grpc.Server, cart *CartHandler, orders *OrderHandler) {
	if cart != nil {
		orderv1.RegisterCartServiceServer(srv, cart)
	}
	if orders != nil {
		orderv1.RegisterOrderServiceServer(srv, orders)
	}
}
