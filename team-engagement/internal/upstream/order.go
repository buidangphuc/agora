// Package upstream holds thin gRPC clients team-engagement uses to consult
// sibling services. It never touches their databases (Rule 3) — cross-service
// reads go through their public gRPC API only.
package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// OrderClient verifies purchases against team-order over gRPC.
type OrderClient struct {
	conn *grpc.ClientConn
	svc  orderv1.OrderServiceClient
}

// NewOrderClient dials addr (e.g. UPSTREAM_ORDER_ADDR). If addr is empty it
// returns (nil, nil) so callers degrade gracefully with verification disabled.
func NewOrderClient(addr string) (*OrderClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial team-order at %s: %w", addr, err)
	}
	return &OrderClient{conn: conn, svc: orderv1.NewOrderServiceClient(conn)}, nil
}

// Close releases the underlying connection.
func (c *OrderClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// VerifyPurchase reports whether buyerID has a completed order (orderID) that
// includes listingID, and returns the order's seller_id. It performs NO DB
// join — it calls team-order.GetOrder and inspects the response.
func (c *OrderClient) VerifyPurchase(ctx context.Context, buyerID, listingID, orderID string) (bool, string, error) {
	if c == nil || c.svc == nil || orderID == "" {
		return false, "", nil
	}
	resp, err := c.svc.GetOrder(ctx, &orderv1.GetOrderRequest{Id: orderID})
	if err != nil {
		return false, "", fmt.Errorf("get order %s: %w", orderID, err)
	}
	order := resp.GetOrder()
	if order == nil {
		return false, "", nil
	}
	if order.GetBuyerId() != buyerID {
		return false, "", nil
	}
	if order.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_COMPLETED {
		return false, order.GetSellerId(), nil
	}
	for _, item := range order.GetItems() {
		if item.GetListingId() == listingID {
			return true, order.GetSellerId(), nil
		}
	}
	return false, order.GetSellerId(), nil
}
