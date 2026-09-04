package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
)

type OrderClient interface {
	GetOrder(ctx context.Context, req *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error)
	// UpdateOrderStatus removed: order transition is now event-carried via
	// PaymentSettled on payment.events (ADR-0009); payment no longer pushes it.
}

func DialOrderService(addr string) (orderv1.OrderServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial order service %s: %w", addr, err)
	}
	return orderv1.NewOrderServiceClient(conn), conn, nil
}
