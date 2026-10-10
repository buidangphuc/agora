// Package upstream holds thin gRPC clients team-engagement uses to consult
// sibling services. It never touches their databases (Rule 3) — cross-service
// reads go through their public gRPC API only.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// Service principal team-engagement presents to team-order. GetOrder requires a principal
// and allows a service principal holding order.read; the caller's user principal
// is deliberately not forwarded (the order may belong to someone else).
const (
	servicePrincipalID     = "service-team-engagement"
	servicePrincipalType   = "service"
	servicePrincipalScopes = "order.read"
)

// serviceMetadata returns the outgoing x-principal-* metadata for team-order calls.
func serviceMetadata() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", servicePrincipalScopes,
	)
}

// servicePrincipalInterceptor replaces any outgoing principal with the service one.
func servicePrincipalInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.NewOutgoingContext(ctx, serviceMetadata()), method, req, reply, cc, opts...)
}

// ErrOrderNotFound means team-order has no such order.
var ErrOrderNotFound = errors.New("order not found")

// OrderClient verifies purchases against team-order over gRPC.
type OrderClient struct {
	conn    *grpc.ClientConn
	svc     orderv1.OrderServiceClient
	timeout time.Duration
}

// NewOrderClient dials addr (e.g. UPSTREAM_ORDER_ADDR). If addr is empty it
// returns (nil, nil) so callers degrade gracefully with verification disabled.
func NewOrderClient(addr string, callTimeout time.Duration) (*OrderClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(servicePrincipalInterceptor))
	if err != nil {
		return nil, fmt.Errorf("dial team-order at %s: %w", addr, err)
	}
	return &OrderClient{conn: conn, svc: orderv1.NewOrderServiceClient(conn), timeout: callTimeout}, nil
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
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	resp, err := c.svc.GetOrder(callCtx, &orderv1.GetOrderRequest{Id: orderID})
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

// GetOrderParties returns the buyer and seller ids of an order, read from team-order
// as team-engagement's service principal (the interceptor replaces any caller
// principal). An unknown order is ErrOrderNotFound; any other upstream failure is
// logged and returned wrapped so the caller can fail closed. It does NOT decide who
// may see the order: the caller compares the ids with its own principal.
func (c *OrderClient) GetOrderParties(ctx context.Context, orderID string) (buyerID, sellerID string, err error) {
	if c == nil || c.svc == nil {
		return "", "", errors.New("order client not configured")
	}
	if orderID == "" {
		return "", "", ErrOrderNotFound
	}
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	resp, err := c.svc.GetOrder(callCtx, &orderv1.GetOrderRequest{Id: orderID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", "", ErrOrderNotFound
		}
		slog.ErrorContext(ctx, "get order parties: cannot read order from team-order",
			slog.String("order_id", orderID), slog.String("grpc_code", status.Code(err).String()), slog.Any("err", err))
		return "", "", fmt.Errorf("get order %s: %w", orderID, err)
	}
	order := resp.GetOrder()
	if order == nil {
		return "", "", ErrOrderNotFound
	}
	return order.GetBuyerId(), order.GetSellerId(), nil
}
