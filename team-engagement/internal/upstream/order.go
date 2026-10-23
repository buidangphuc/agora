package upstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

const (
	servicePrincipalID    = "service-team-engagement"
	servicePrincipalScope = "order.read"
)

var ErrOrderNotFound = errors.New("order not found")

var ErrOrderClientNotConfigured = errors.New("order client not configured")

var ErrOrderIDRequired = errors.New("order id required")

type OrderClient struct {
	conn *grpc.ClientConn
	svc  orderv1.OrderServiceClient
}

func NewOrderClient(addr string) (*OrderClient, error) {
	conn, err := sdkupstream.NewClient(addr, sdkupstream.AsService(servicePrincipalID, servicePrincipalScope))
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

func (c *OrderClient) VerifyPurchase(ctx context.Context, buyerID, listingID, orderID string) (bool, string, error) {
	if c == nil || c.svc == nil {
		return false, "", ErrOrderClientNotConfigured
	}
	if orderID == "" {
		return false, "", ErrOrderIDRequired
	}
	resp, err := c.svc.GetOrder(ctx, &orderv1.GetOrderRequest{Id: orderID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, "", nil
		}
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

func (c *OrderClient) GetOrderParties(ctx context.Context, orderID string) (buyerID, sellerID string, err error) {
	if c == nil || c.svc == nil {
		return "", "", errors.New("order client not configured")
	}
	if orderID == "" {
		return "", "", ErrOrderNotFound
	}
	resp, err := c.svc.GetOrder(ctx, &orderv1.GetOrderRequest{Id: orderID})
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

var ErrReturnNotFound = errors.New("return not found")

func (c *OrderClient) GetReturn(ctx context.Context, returnID string) (string, orderv1.ReturnStatus, bool, error) {
	if c == nil || c.svc == nil {
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, ErrOrderClientNotConfigured
	}
	if returnID == "" {
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, ErrReturnNotFound
	}
	resp, err := c.svc.GetReturnRequest(ctx, &orderv1.GetReturnRequestRequest{Id: returnID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, ErrReturnNotFound
		}
		slog.ErrorContext(ctx, "get return: cannot read return from team-order",
			slog.String("return_id", returnID), slog.String("grpc_code", status.Code(err).String()), slog.Any("err", err))
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, fmt.Errorf("get return %s: upstream unavailable (%s)", returnID, status.Code(err))
	}
	ret := resp.GetReturnRequest()
	if ret == nil {
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, ErrReturnNotFound
	}
	return ret.GetOrderId(), ret.GetStatus(), ret.GetCanEscalate(), nil
}
