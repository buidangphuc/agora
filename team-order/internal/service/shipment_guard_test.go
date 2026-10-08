package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

func shipmentRig(t *testing.T, st repository.OrderStatus) (*service.OrderService, *repository.InMemoryOrderRepository, *repository.InMemoryShipmentRepository, *repository.InMemoryOutboxRepository) {
	t.Helper()
	orders := repository.NewInMemoryOrderRepository()
	if _, err := orders.CreateOrder(context.Background(), repository.Order{ID: "ord_s", BuyerID: "b", SellerID: "s", Status: st}); err != nil {
		t.Fatal(err)
	}
	outbox := repository.NewInMemoryOutboxRepository()
	ships := repository.NewInMemoryShipmentRepository(
		repository.WithShipmentOutbox(events.BuildShippedOutboxRow), repository.WithInMemoryShipmentOutbox(outbox))
	return service.NewOrderService(orders, nil, nil, ships, nil, nil, nil), orders, ships, outbox
}

func TestCreateShipment_CancelledOrderIsRefused(t *testing.T) {
	for _, st := range []repository.OrderStatus{repository.OrderStatusCancelled, repository.OrderStatusCompleted, repository.OrderStatusShipped} {
		svc, orders, ships, outbox := shipmentRig(t, st)
		if _, err := svc.CreateShipment(context.Background(), "ord_s", "GHN", "TRK"); !errors.Is(err, service.ErrInvalidStatus) {
			t.Fatalf("%v: want ErrInvalidStatus, got %v", st, err)
		}
		if _, err := ships.GetShipmentByOrderID(context.Background(), "ord_s"); !errors.Is(err, repository.ErrShipmentNotFound) {
			t.Fatalf("%v: no shipment may exist, got %v", st, err)
		}
		if o, _ := orders.GetOrder(context.Background(), "ord_s"); o.Status != st {
			t.Fatalf("order must be unchanged: %v -> %v", st, o.Status)
		}
		if len(outbox.EnqueuedRows()) != 0 {
			t.Fatalf("%v: no OrderShipped row may be written", st)
		}
	}
}

func TestCreateShipment_PaidOrderShips(t *testing.T) {
	for _, st := range []repository.OrderStatus{repository.OrderStatusPaid, repository.OrderStatusPending} {
		svc, orders, ships, outbox := shipmentRig(t, st)
		created, err := svc.CreateShipment(context.Background(), "ord_s", "GHN", "TRK-1")
		if err != nil {
			t.Fatalf("%v: %v", st, err)
		}
		if got, err := ships.GetShipmentByOrderID(context.Background(), "ord_s"); err != nil || got.ID != created.ID {
			t.Fatalf("%v: shipment missing: %v", st, err)
		}
		o, _ := orders.GetOrder(context.Background(), "ord_s")
		if o.Status != repository.OrderStatusShipped || o.TrackingNumber != "TRK-1" {
			t.Fatalf("%v: order %+v", st, o)
		}
		rows := outbox.EnqueuedRows()
		if len(rows) != 1 || rows[0].AggregateID != "ord_s" || rows[0].EventType != events.OrderShippedEventType {
			t.Fatalf("%v: want one OrderShipped row, got %+v", st, rows)
		}
		if _, err := svc.CreateShipment(context.Background(), "ord_s", "GHN", "TRK-2"); !errors.Is(err, service.ErrInvalidStatus) {
			t.Fatalf("%v: a second shipment must conflict, got %v", st, err)
		}
	}
}
