package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
)

// Service principal team-payment presents to team-order. GetOrder requires a principal
// and allows a service principal holding order.read; the caller's user principal
// is deliberately not forwarded (the order may belong to someone else).
const (
	servicePrincipalID     = "service-team-payment"
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

type OrderClient interface {
	GetOrder(ctx context.Context, req *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error)
	// UpdateOrderStatus removed: order transition is now event-carried via
	// PaymentSettled on payment.events (ADR-0009); payment no longer pushes it.
}

func DialOrderService(addr string) (orderv1.OrderServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(servicePrincipalInterceptor))
	if err != nil {
		return nil, nil, fmt.Errorf("dial order service %s: %w", addr, err)
	}
	return orderv1.NewOrderServiceClient(conn), conn, nil
}
