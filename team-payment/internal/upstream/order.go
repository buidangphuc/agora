package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
)

const (
	servicePrincipalID    = "service-team-payment"
	servicePrincipalScope = "order.read"
)

type OrderClient interface {
	GetOrder(ctx context.Context, req *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error)
}

func DialOrderService(addr string) (orderv1.OrderServiceClient, *grpc.ClientConn, error) {
	conn, err := sdkupstream.NewClient(addr, sdkupstream.AsService(servicePrincipalID, servicePrincipalScope))
	if err != nil {
		return nil, nil, fmt.Errorf("dial order service %s: %w", addr, err)
	}
	return orderv1.NewOrderServiceClient(conn), conn, nil
}
