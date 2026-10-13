package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
)

func TestCreateShipment_CancelledOrderIsFailedPrecondition(t *testing.T) {
	orders := store.NewInMemoryOrderRepository()
	if _, err := orders.CreateOrder(context.Background(), store.Order{ID: "ord_c", BuyerID: "buyer_1", SellerID: "seller_1", Status: store.OrderStatusCancelled}); err != nil {
		t.Fatal(err)
	}
	ships := store.NewInMemoryShipmentRepository()
	h := rpc.NewOrderHandler(order.NewOrderService(orders, nil, nil, ships, nil, nil, nil), nil, nil)
	_, err := h.CreateShipment(incomingPrincipalCtx("seller_1", "seller"), &orderv1.CreateShipmentRequest{OrderId: "ord_c", Carrier: "GHN"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if _, err := ships.GetShipmentByOrderID(context.Background(), "ord_c"); err == nil {
		t.Fatal("no shipment may exist")
	}
	if o, _ := orders.GetOrder(context.Background(), "ord_c"); o.Status != store.OrderStatusCancelled {
		t.Fatalf("order must stay Cancelled, got %v", o.Status)
	}
}
