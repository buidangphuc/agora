package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

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

func TestOrderService_CalculateShippingFee(t *testing.T) {
	s := service.NewOrderService(nil, nil, nil, nil, nil, nil, nil)

	fee, isFree, _ := s.CalculateShippingFee("Hà Nội", 600000)
	if !isFree || fee != 0 {
		t.Errorf("expected free shipping, got fee %d", fee)
	}

	fee, isFree, _ = s.CalculateShippingFee("Hà Nội", 200000)
	if isFree || fee != 20000 {
		t.Errorf("expected 20000, got fee %d", fee)
	}

	fee, isFree, _ = s.CalculateShippingFee("Đà Nẵng", 200000)
	if isFree || fee != 35000 {
		t.Errorf("expected 35000, got fee %d", fee)
	}
}

func TestOrderService_GetOrder(t *testing.T) {
	repo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_1": {ID: "ord_1", BuyerID: "buyer_1", TotalAmount: 100000, Status: repository.OrderStatusPending},
		},
	}
	s := service.NewOrderService(repo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	o, err := s.GetOrder(ctx, "ord_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.ID != "ord_1" {
		t.Errorf("expected ord_1, got %s", o.ID)
	}

	_, err = s.GetOrder(ctx, "not_found")
	if err == nil {
		t.Errorf("expected not found error")
	}
}

func TestOrderService_CancelOrder(t *testing.T) {
	repo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_cancel": {ID: "ord_cancel", BuyerID: "buyer_1", Status: repository.OrderStatusPending},
		},
	}
	s := service.NewOrderService(repo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	cancelled, err := s.CancelOrder(ctx, "ord_cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cancelled.Status != repository.OrderStatusCancelled {
		t.Errorf("expected status cancelled, got %v", cancelled.Status)
	}
}

func TestOrderService_ReturnRequest(t *testing.T) {
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_delivered": {
				ID:          "ord_delivered",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 500000,
				Status:      repository.OrderStatusCompleted,
			},
			"ord_pending": {
				ID:          "ord_pending",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 200000,
				Status:      repository.OrderStatusPending,
			},
		},
	}
	returnRepo := repository.NewInMemoryReturnRepository()
	s := service.NewOrderService(orderRepo, nil, returnRepo, nil, nil, nil, nil)
	ctx := context.Background()

	t.Run("Create Return - Success", func(t *testing.T) {
		ret, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_delivered", "Hàng bị lỗi rách bao bì", 500000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ret.ID == "" {
			t.Errorf("expected generated return ID")
		}
		if ret.Status != repository.ReturnStatusPending {
			t.Errorf("expected status PENDING, got %d", ret.Status)
		}
		if ret.RefundAmount != 500000 {
			t.Errorf("expected refund 500000, got %d", ret.RefundAmount)
		}

		// Get Return
		got, err := s.GetReturnRequest(ctx, ret.ID)
		if err != nil {
			t.Fatalf("GetReturnRequest failed: %v", err)
		}
		if got.Reason != "Hàng bị lỗi rách bao bì" {
			t.Errorf("unexpected reason: %s", got.Reason)
		}
	})

	t.Run("Create Return - Unauthorized Buyer", func(t *testing.T) {
		_, err := s.CreateReturnRequest(ctx, "other_buyer", "ord_delivered", "Reason", 100000)
		if err != service.ErrUnauthorizedReturn {
			t.Errorf("expected ErrUnauthorizedReturn, got %v", err)
		}
	})

	t.Run("Create Return - Invalid Order Status", func(t *testing.T) {
		_, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_pending", "Reason", 100000)
		if err == nil {
			t.Errorf("expected error for returning pending order, got nil")
		}
	})

	t.Run("Create Return - Refund Exceeds Total", func(t *testing.T) {
		_, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_delivered", "Reason", 900000)
		if err == nil {
			t.Errorf("expected refund amount error, got nil")
		}
	})

	t.Run("Update Return Status - Valid Transitions", func(t *testing.T) {
		ret, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_delivered", "Đổi trả hàng", 200000)
		if err != nil {
			t.Fatalf("create return failed: %v", err)
		}

		// Pending -> Approved
		approved, err := s.UpdateReturnStatus(ctx, ret.ID, repository.ReturnStatusApproved)
		if err != nil {
			t.Fatalf("Update to Approved failed: %v", err)
		}
		if approved.Status != repository.ReturnStatusApproved {
			t.Errorf("expected APPROVED, got %d", approved.Status)
		}

		// Approved -> Refunded
		refunded, err := s.UpdateReturnStatus(ctx, ret.ID, repository.ReturnStatusRefunded)
		if err != nil {
			t.Fatalf("Update to Refunded failed: %v", err)
		}
		if refunded.Status != repository.ReturnStatusRefunded {
			t.Errorf("expected REFUNDED, got %d", refunded.Status)
		}

		// Refunded -> Approved (Invalid, terminal state)
		_, err = s.UpdateReturnStatus(ctx, ret.ID, repository.ReturnStatusApproved)
		if err == nil {
			t.Errorf("expected error updating terminal state, got nil")
		}
	})
}

func TestOrderService_Shipment(t *testing.T) {
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]repository.Order{
			"ord_to_ship": {
				ID:          "ord_to_ship",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 300000,
				Status:      repository.OrderStatusPaid,
			},
		},
	}
	shipmentRepo := repository.NewInMemoryShipmentRepository()
	s := service.NewOrderService(orderRepo, nil, nil, shipmentRepo, nil, nil, nil)
	ctx := context.Background()

	t.Run("Create Shipment", func(t *testing.T) {
		sh, err := s.CreateShipment(ctx, "ord_to_ship", "GHN", "GHN-VN-12345")
		if err != nil {
			t.Fatalf("CreateShipment failed: %v", err)
		}
		if sh.ID == "" {
			t.Errorf("expected generated shipment ID")
		}
		if sh.Carrier != "GHN" || sh.TrackingCode != "GHN-VN-12345" {
			t.Errorf("unexpected carrier/code: %s, %s", sh.Carrier, sh.TrackingCode)
		}
		if len(sh.Checkpoints) != 1 {
			t.Errorf("expected 1 initial checkpoint, got %d", len(sh.Checkpoints))
		}

		// Order should now be marked SHIPPED
		ord, _ := orderRepo.GetOrder(ctx, "ord_to_ship")
		if ord.Status != repository.OrderStatusShipped {
			t.Errorf("expected order status SHIPPED, got %v", ord.Status)
		}
		if ord.TrackingNumber != "GHN-VN-12345" {
			t.Errorf("expected order tracking number GHN-VN-12345, got %s", ord.TrackingNumber)
		}
	})

	t.Run("Get Shipment Tracking", func(t *testing.T) {
		// By tracking code
		byCode, err := s.GetShipmentTracking(ctx, "GHN-VN-12345", "", "")
		if err != nil {
			t.Fatalf("GetShipmentTracking by code failed: %v", err)
		}
		if byCode.TrackingCode != "GHN-VN-12345" {
			t.Errorf("expected GHN-VN-12345, got %s", byCode.TrackingCode)
		}

		// By order id
		byOrder, err := s.GetShipmentTracking(ctx, "", "ord_to_ship", "")
		if err != nil {
			t.Fatalf("GetShipmentTracking by order failed: %v", err)
		}
		if byOrder.OrderID != "ord_to_ship" {
			t.Errorf("expected ord_to_ship, got %s", byOrder.OrderID)
		}

		// Add checkpoint
		cp, err := s.AddShipmentCheckpoint(ctx, byCode.ID, "Bưu cục Cầu Giấy", "Đang giao hàng tới người nhận")
		if err != nil {
			t.Fatalf("AddShipmentCheckpoint failed: %v", err)
		}
		if cp.Location != "Bưu cục Cầu Giấy" {
			t.Errorf("expected location Bưu cục Cầu Giấy, got %s", cp.Location)
		}

		// Check updated tracking
		updated, _ := s.GetShipmentTracking(ctx, "GHN-VN-12345", "", "")
		if len(updated.Checkpoints) != 2 {
			t.Errorf("expected 2 checkpoints, got %d", len(updated.Checkpoints))
		}
	})
}
