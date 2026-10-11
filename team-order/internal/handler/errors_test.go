package handler_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// dbErrRepo fails every read with a driver-looking error.
type dbErrRepo struct{ mockOrderServiceRepo }

const driverText = `ERROR: relation "orders" does not exist (SQLSTATE 42P01) host=10.0.0.5`

func (*dbErrRepo) GetOrder(context.Context, string) (repository.Order, error) {
	return repository.Order{}, errors.New(driverText)
}

func (*dbErrRepo) ListBuyerOrders(context.Context, string, int32) ([]repository.Order, error) {
	return nil, errors.New(driverText)
}

func TestHandlerInternalErrorsAreGenericAndDoNotLeakCause(t *testing.T) {
	svc := service.NewOrderService(&dbErrRepo{}, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	_, err := h.GetOrder(ctx, &orderv1.GetOrderRequest{Id: "o1"})
	assertGenericInternal(t, err, "get order failed")

	_, err = h.ListBuyerOrders(ctx, &orderv1.ListBuyerOrdersRequest{})
	assertGenericInternal(t, err, "list buyer orders failed")

	_, err = h.CancelOrder(ctx, &orderv1.CancelOrderRequest{Id: "o1"})
	assertGenericInternal(t, err, "get order failed")
}

func assertGenericInternal(t *testing.T, err error, want string) {
	t.Helper()
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal (err=%v)", status.Code(err), err)
	}
	msg := status.Convert(err).Message()
	if msg != want {
		t.Fatalf("message = %q, want %q", msg, want)
	}
	if strings.Contains(msg, "SQLSTATE") || strings.Contains(msg, "10.0.0.5") {
		t.Fatalf("driver text leaked to client: %q", msg)
	}
}

func TestHandlerMappedErrorsUseStableMessages(t *testing.T) {
	repo := &mockOrderServiceRepo{orders: map[string]repository.Order{
		"o1": {ID: "o1", BuyerID: "buyer_1", SellerID: "seller_1", Status: repository.OrderStatusCancelled},
	}}
	svc := service.NewOrderService(repo, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)

	// Cancelling an already-cancelled order is a client error, not INTERNAL, and
	// the message does not echo internal status names.
	_, err := h.CancelOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CancelOrderRequest{Id: "o1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("cancel code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	if got := status.Convert(err).Message(); got != "order cannot be cancelled in its current status" {
		t.Fatalf("cancel message = %q", got)
	}

	// Seller status update with a disallowed transition: stable message.
	_, err = h.UpdateOrderStatus(incomingPrincipalCtx("seller_1", "seller"), &orderv1.UpdateOrderStatusRequest{
		Id: "o1", Status: orderv1.OrderStatus_ORDER_STATUS_SHIPPED,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("update code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	if got := status.Convert(err).Message(); got != "invalid order status transition" {
		t.Fatalf("update message = %q", got)
	}
}
