package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	orderv1 "github.com/buidangphuc/team-promotion/generated/platform/order/v1"
)

const (
	orderServicePrincipalID    = "service-team-promotion"
	orderServicePrincipalScope = "order.read"
)

type OrderClient struct {
	conn *grpc.ClientConn
	svc  orderv1.OrderServiceClient
}

func NewOrderClient(addr string, timeout time.Duration) (*OrderClient, error) {
	conn, err := sdkupstream.NewClient(addr,
		sdkupstream.AsService(orderServicePrincipalID, orderServicePrincipalScope),
		sdkupstream.WithTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("dial team-order at %s: %w", addr, err)
	}
	return &OrderClient{conn: conn, svc: orderv1.NewOrderServiceClient(conn)}, nil
}

func (c *OrderClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *OrderClient) HasCompletedOrder(ctx context.Context, buyerID string) (bool, error) {
	if c == nil || c.svc == nil {
		return false, status.Error(codes.Unavailable, "order client not configured")
	}
	resp, err := c.svc.HasCompletedOrder(ctx, &orderv1.HasCompletedOrderRequest{BuyerId: buyerID})
	if err != nil {
		slog.WarnContext(ctx, "has completed order check failed", "grpc_code", status.Code(err).String())
		return false, status.Error(codes.Unavailable, "order service check failed")
	}
	return resp.GetHasCompleted(), nil
}
