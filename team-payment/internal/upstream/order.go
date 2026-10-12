// Package upstream is team-payment's client of team-order.
package upstream

import (
	"context"

	"google.golang.org/grpc"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/upstream"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
)

// Service principal team-payment presents to team-order. GetOrder requires a principal
// and allows a service principal holding order.read; the caller's user principal
// is deliberately not forwarded (the order may belong to someone else).
const (
	servicePrincipalID    = "service-team-payment"
	servicePrincipalScope = "order.read"
)

// OrderClient is the slice of team-order's client the payment service uses.
// UpdateOrderStatus is gone: the order transition is event-carried via
// PaymentSettled on payment.events (ADR-0009).
type OrderClient interface {
	GetOrder(ctx context.Context, req *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error)
}

// orderClient replaces whatever principal the incoming request carried with the
// service principal on every call.
type orderClient struct{ cli orderv1.OrderServiceClient }

// NewOrderClient wraps conn (from the SDK upstream.Dial) as an OrderClient that
// calls team-order as team-payment's service principal.
func NewOrderClient(conn grpc.ClientConnInterface) OrderClient {
	return orderClient{cli: orderv1.NewOrderServiceClient(conn)}
}

// GetOrder calls team-order's GetOrder as the service principal.
func (c orderClient) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return c.cli.GetOrder(upstream.WithServicePrincipal(ctx, servicePrincipalID, servicePrincipalScope), req, opts...)
}
