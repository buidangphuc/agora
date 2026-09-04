package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/interceptor"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

// fakeFlags is a hand-rolled Evaluator for the checkout kill-switch tests.
type fakeFlags struct{ enabled bool }

func (f fakeFlags) BooleanEnabled(_ context.Context, _ string, _ bool) bool { return f.enabled }

type mockOrderServiceRepo struct {
	orders map[string]repository.Order
}

func (m *mockOrderServiceRepo) CreateOrder(ctx context.Context, o repository.Order) (repository.Order, error) {
	if m.orders == nil {
		m.orders = make(map[string]repository.Order)
	}
	m.orders[o.ID] = o
	return o, nil
}

func (m *mockOrderServiceRepo) GetOrder(ctx context.Context, id string) (repository.Order, error) {
	if o, ok := m.orders[id]; ok {
		return o, nil
	}
	return repository.Order{}, repository.ErrOrderNotFound
}

func (m *mockOrderServiceRepo) ListBuyerOrders(ctx context.Context, buyerID string, statusFilter int32) ([]repository.Order, error) {
	var list []repository.Order
	for _, o := range m.orders {
		if o.BuyerID == buyerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrderServiceRepo) ListSellerOrders(ctx context.Context, sellerID string, statusFilter int32) ([]repository.Order, error) {
	var list []repository.Order
	for _, o := range m.orders {
		if o.SellerID == sellerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrderServiceRepo) UpdateOrderStatus(ctx context.Context, id string, status repository.OrderStatus, trackingNumber string) (repository.Order, error) {
	if o, ok := m.orders[id]; ok {
		o.Status = status
		o.TrackingNumber = trackingNumber
		m.orders[id] = o
		return o, nil
	}
	return repository.Order{}, repository.ErrOrderNotFound
}

func incomingPrincipalCtx(userID, userType string) context.Context {
	p := &commonv1.Principal{
		Id:     userID,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"order.read", "order.write"},
	}
	return interceptor.ContextWithPrincipal(context.Background(), p)
}

func TestOrderHandler_CalculateShippingFee(t *testing.T) {
	svc := service.NewOrderService(nil, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)
	ctx := context.Background()

	t.Run("Free Shipping Over 500k", func(t *testing.T) {
		res, err := h.CalculateShippingFee(ctx, &orderv1.CalculateShippingFeeRequest{
			City:          "Hà Nội",
			ItemsSubtotal: 600000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsFreeShipping {
			t.Errorf("expected free shipping")
		}
		if res.ShippingFee != 0 {
			t.Errorf("expected 0 fee, got %d", res.ShippingFee)
		}
	})

	t.Run("Standard City Shipping", func(t *testing.T) {
		res, err := h.CalculateShippingFee(ctx, &orderv1.CalculateShippingFeeRequest{
			City:          "Hà Nội",
			ItemsSubtotal: 200000,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsFreeShipping {
			t.Errorf("expected not free shipping")
		}
		if res.ShippingFee != 20000 {
			t.Errorf("expected 20000, got %d", res.ShippingFee)
		}
	})
}

func TestOrderHandler_CreateOrder_KillSwitchOff(t *testing.T) {
	// Flag OFF → checkout rejected before the saga runs.
	svc := service.NewOrderService(nil, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil, handler.WithFeatureFlags(fakeFlags{enabled: false}))
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	_, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
	if msg := status.Convert(err).Message(); !strings.Contains(msg, "unavailable") {
		t.Fatalf("expected checkout-unavailable message, got %q", msg)
	}
}

func TestOrderHandler_CreateOrder_KillSwitchOn(t *testing.T) {
	// Flag ON → gate passes, request reaches the service. With an empty cart the
	// service returns ErrEmptyCart, proving the kill-switch let it through (a
	// distinct precondition message from the kill-switch one).
	cartRepo := repository.NewInMemoryCartRepository()
	svc := service.NewOrderService(
		repository.NewInMemoryOrderRepository(), cartRepo, nil, nil, nil, nil, nil,
	)
	h := handler.NewOrderHandler(svc, nil, nil, handler.WithFeatureFlags(fakeFlags{enabled: true}))
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	_, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition (empty cart), got %v", err)
	}
	if msg := status.Convert(err).Message(); !strings.Contains(msg, "cart is empty") {
		t.Fatalf("expected empty-cart message (gate passed), got %q", msg)
	}
}

func TestOrderHandler_CreateOrder_NoFlagsFailsOpen(t *testing.T) {
	// No evaluator wired → treated as enabled (fail-open); request reaches the
	// service and hits the empty-cart precondition, not the kill-switch.
	cartRepo := repository.NewInMemoryCartRepository()
	svc := service.NewOrderService(
		repository.NewInMemoryOrderRepository(), cartRepo, nil, nil, nil, nil, nil,
	)
	h := handler.NewOrderHandler(svc, nil, nil)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	_, err := h.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if msg := status.Convert(err).Message(); !strings.Contains(msg, "cart is empty") {
		t.Fatalf("expected empty-cart message (fail-open), got %q", msg)
	}
}

func TestOrderHandler_GetSagaState(t *testing.T) {
	repo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_123": {
				ID:          "ord_123",
				BuyerID:     "buyer_1",
				TotalAmount: 100000,
				Status:      repository.OrderStatusPending,
			},
		},
	}
	svc := service.NewOrderService(repo, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	res, err := h.GetSagaState(ctx, &orderv1.GetSagaStateRequest{OrderId: "ord_123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Steps) != 4 {
		t.Errorf("expected 4 saga steps, got %d", len(res.Steps))
	}
}

func TestOrderHandler_ForceFailSaga(t *testing.T) {
	repo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_123": {
				ID:          "ord_123",
				BuyerID:     "buyer_1",
				TotalAmount: 100000,
				Status:      repository.OrderStatusPending,
			},
		},
	}
	svc := service.NewOrderService(repo, nil, nil, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)
	ctx := incomingPrincipalCtx("buyer_1", "buyer")

	res, err := h.ForceFailSaga(ctx, &orderv1.ForceFailSagaRequest{OrderId: "ord_123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success true")
	}
}

func TestOrderHandler_RMA(t *testing.T) {
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_comp": {
				ID:          "ord_comp",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 300000,
				Status:      repository.OrderStatusCompleted,
			},
		},
	}
	returnRepo := repository.NewInMemoryReturnRepository()
	svc := service.NewOrderService(orderRepo, nil, returnRepo, nil, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)

	buyerCtx := incomingPrincipalCtx("buyer_1", "buyer")
	sellerCtx := incomingPrincipalCtx("seller_1", "seller")

	// 1. Create Return Request
	createResp, err := h.CreateReturnRequest(buyerCtx, &orderv1.CreateReturnRequestRequest{
		OrderId:      "ord_comp",
		Reason:       "Hàng nhận được không đúng mẫu",
		RefundAmount: 300000,
	})
	if err != nil {
		t.Fatalf("CreateReturnRequest failed: %v", err)
	}
	if createResp.GetReturnRequest().GetId() == "" {
		t.Errorf("expected return request ID")
	}
	if createResp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Errorf("expected PENDING status")
	}

	returnID := createResp.GetReturnRequest().GetId()

	// 2. Get Return Request (as Buyer)
	getResp, err := h.GetReturnRequest(buyerCtx, &orderv1.GetReturnRequestRequest{Id: returnID})
	if err != nil {
		t.Fatalf("GetReturnRequest failed: %v", err)
	}
	if getResp.GetReturnRequest().GetReason() != "Hàng nhận được không đúng mẫu" {
		t.Errorf("unexpected reason: %s", getResp.GetReturnRequest().GetReason())
	}

	// 3. Update Return Status (as Seller -> APPROVED)
	updateResp, err := h.UpdateReturnStatus(sellerCtx, &orderv1.UpdateReturnStatusRequest{
		Id:     returnID,
		Status: orderv1.ReturnStatus_RETURN_STATUS_APPROVED,
	})
	if err != nil {
		t.Fatalf("UpdateReturnStatus failed: %v", err)
	}
	if updateResp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
		t.Errorf("expected APPROVED, got %v", updateResp.GetReturnRequest().GetStatus())
	}
}

func TestOrderHandler_Shipment(t *testing.T) {
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_ship": {
				ID:          "ord_ship",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 250000,
				Status:      repository.OrderStatusPaid,
			},
		},
	}
	shipmentRepo := repository.NewInMemoryShipmentRepository()
	svc := service.NewOrderService(orderRepo, nil, nil, shipmentRepo, nil, nil, nil)
	h := handler.NewOrderHandler(svc, nil, nil)

	sellerCtx := incomingPrincipalCtx("seller_1", "seller")

	// 1. Create Shipment
	shipResp, err := h.CreateShipment(sellerCtx, &orderv1.CreateShipmentRequest{
		OrderId:      "ord_ship",
		Carrier:      "SPX",
		TrackingCode: "SPX-VN-999888",
	})
	if err != nil {
		t.Fatalf("CreateShipment failed: %v", err)
	}
	if shipResp.GetShipment().GetTrackingCode() != "SPX-VN-999888" {
		t.Errorf("expected tracking code SPX-VN-999888, got %s", shipResp.GetShipment().GetTrackingCode())
	}
	if len(shipResp.GetShipment().GetCheckpoints()) != 1 {
		t.Errorf("expected 1 initial checkpoint")
	}

	// 2. Get Shipment Tracking
	trackResp, err := h.GetShipmentTracking(context.Background(), &orderv1.GetShipmentTrackingRequest{
		TrackingCode: "SPX-VN-999888",
	})
	if err != nil {
		t.Fatalf("GetShipmentTracking failed: %v", err)
	}
	if trackResp.GetShipment().GetOrderId() != "ord_ship" {
		t.Errorf("expected orderId ord_ship, got %s", trackResp.GetShipment().GetOrderId())
	}
}
