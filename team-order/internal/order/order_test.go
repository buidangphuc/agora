package order_test

import (
	"context"
	"errors"
	"testing"
	"time"

	outboxsdk "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

type mockOrderServiceRepo struct {
	orders map[string]store.Order
}

func (m *mockOrderServiceRepo) CreateOrder(_ context.Context, o store.Order) (store.Order, error) {
	if m.orders == nil {
		m.orders = make(map[string]store.Order)
	}
	m.orders[o.ID] = o
	return o, nil
}

func (m *mockOrderServiceRepo) GetOrder(_ context.Context, id string) (store.Order, error) {
	if o, ok := m.orders[id]; ok {
		return o, nil
	}
	return store.Order{}, store.ErrOrderNotFound
}

func (m *mockOrderServiceRepo) ListBuyerOrders(_ context.Context, buyerID string, _ int32) ([]store.Order, error) {
	var list []store.Order
	for _, o := range m.orders {
		if o.BuyerID == buyerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrderServiceRepo) ListSellerOrders(_ context.Context, sellerID string, _ int32) ([]store.Order, error) {
	var list []store.Order
	for _, o := range m.orders {
		if o.SellerID == sellerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrderServiceRepo) UpdateOrderStatusFrom(_ context.Context, id string, to store.OrderStatus, from []store.OrderStatus, trackingNumber string, _ ...store.StatusOption) (store.Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return store.Order{}, store.ErrOrderNotFound
	}
	allowed := false
	for _, st := range from {
		if o.Status == st {
			allowed = true
		}
	}
	if !allowed {
		return store.Order{}, store.ErrStatusConflict
	}
	o.Status = to
	if trackingNumber != "" {
		o.TrackingNumber = trackingNumber
	}
	if to == store.OrderStatusPaid {
		now := time.Now()
		o.PaidAt = &now
	}
	m.orders[id] = o
	return o, nil
}

func etaService(t *testing.T, now time.Time) *order.OrderService {
	return newSvc(t, nil, nil, nil, nil, nil, nil, nil, order.WithClock(func() time.Time { return now }))
}

func TestOrderService_CalculateShippingFee(t *testing.T) {
	s := newSvc(t, nil, nil, nil, nil, nil, nil, nil)

	fee, isFree, _, _, _ := s.CalculateShippingFee("Hà Nội", 600000)
	if !isFree || fee != 0 {
		t.Errorf("expected free shipping, got fee %d", fee)
	}

	fee, isFree, _, _, _ = s.CalculateShippingFee("Hà Nội", 200000)
	if isFree || fee != 20000 {
		t.Errorf("expected 20000, got fee %d", fee)
	}

	fee, isFree, _, _, _ = s.CalculateShippingFee("Đà Nẵng", 200000)
	if isFree || fee != 35000 {
		t.Errorf("expected 35000, got fee %d", fee)
	}
}

func TestShippingETAHoChiMinhTwoDays(t *testing.T) {
	now := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	_, _, _, days, at := etaService(t, now).CalculateShippingFee("Hồ Chí Minh", 300000)
	if days != 2 {
		t.Errorf("Address and ETA: Hồ Chí Minh is delivered in 2 days: days=%d", days)
	}
	if want := time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Errorf("Address and ETA: Hồ Chí Minh is delivered in 2 days: at=%v want %v", at, want)
	}
}

func TestShippingETAHaNoiTwoDays(t *testing.T) {
	now := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	_, _, _, days, at := etaService(t, now).CalculateShippingFee("Hà Nội", 300000)
	if days != 2 {
		t.Errorf("Address and ETA: Hà Nội is delivered in 2 days: days=%d", days)
	}
	if want := time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Errorf("Address and ETA: Hà Nội is delivered in 2 days: at=%v want %v", at, want)
	}
}

func TestShippingETAOtherCityFourDays(t *testing.T) {
	now := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	_, _, _, days, at := etaService(t, now).CalculateShippingFee("Đà Nẵng", 300000)
	if days != 4 {
		t.Errorf("Address and ETA: Any other city is delivered in 4 days: days=%d", days)
	}
	if want := time.Date(2026, 10, 20, 5, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Errorf("Address and ETA: Any other city is delivered in 4 days: at=%v want %v", at, want)
	}
}

func TestShippingETAIsHoChiMinhCalendarDate(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)
	_, _, _, _, at := etaService(t, now).CalculateShippingFee("Đà Nẵng", 300000)
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load tz: %v", err)
	}
	if got := at.In(loc).Format("02/01/2006"); got != "21/10/2026" {
		t.Errorf("Address and ETA: The delivery date is a calendar date in Asia/Ho_Chi_Minh: got %s want 21/10/2026", got)
	}
}

func TestShippingETAIndependentOfFreeShipping(t *testing.T) {
	now := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	fee, isFree, _, days, at := etaService(t, now).CalculateShippingFee("Đà Nẵng", 600000)
	if fee != 0 || !isFree {
		t.Errorf("Address and ETA: Free shipping does not change the delivery time: fee=%d free=%v", fee, isFree)
	}
	if days != 4 || at.IsZero() {
		t.Errorf("Address and ETA: Free shipping does not change the delivery time: days=%d at=%v", days, at)
	}
}

func TestShippingFeeAmountsUnchanged(t *testing.T) {
	s := etaService(t, time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC))
	cases := []struct {
		city     string
		subtotal int64
		fee      int64
		free     bool
	}{
		{"Hà Nội", 200000, 20000, false},
		{"Đà Nẵng", 200000, 35000, false},
		{"Đà Nẵng", 500000, 0, true},
	}
	for _, c := range cases {
		fee, free, _, _, _ := s.CalculateShippingFee(c.city, c.subtotal)
		if fee != c.fee || free != c.free {
			t.Errorf("Address and ETA: Fee amounts are unchanged: %s/%d got fee=%d free=%v want fee=%d free=%v", c.city, c.subtotal, fee, free, c.fee, c.free)
		}
	}
}

func TestOrderService_GetOrder(t *testing.T) {
	repo := &mockOrderServiceRepo{
		orders: map[string]store.Order{
			"ord_1": {ID: "ord_1", BuyerID: "buyer_1", TotalAmount: 100000, Status: store.OrderStatusPending},
		},
	}
	s := newSvc(t, repo, nil, nil, nil, nil, nil, nil)
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
		orders: map[string]store.Order{
			"ord_cancel": {ID: "ord_cancel", BuyerID: "buyer_1", Status: store.OrderStatusPending},
		},
	}
	s := newSvc(t, repo, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	cancelled, err := s.CancelOrder(ctx, "ord_cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cancelled.Status != store.OrderStatusCancelled {
		t.Errorf("expected status cancelled, got %v", cancelled.Status)
	}
}

func TestOrderService_ReturnRequest(t *testing.T) {
	paidAt := time.Now()
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]store.Order{
			"ord_paid_online": {
				ID:          "ord_paid_online",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 500000,
				Status:      store.OrderStatusCompleted,
				PaidAt:      &paidAt,
			},
			"ord_delivered": {
				ID:          "ord_delivered",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 500000,
				Status:      store.OrderStatusCompleted,
			},
			"ord_pending": {
				ID:          "ord_pending",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 200000,
				Status:      store.OrderStatusPending,
			},
		},
	}
	returnRepo := fake.NewInMemoryReturnRepository()
	s := newSvc(t, orderRepo, nil, returnRepo, nil, nil, nil, nil)
	ctx := context.Background()

	t.Run("Create Return - Success", func(t *testing.T) {
		ret, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_delivered", "Hàng bị lỗi rách bao bì", 500000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ret.ID == "" {
			t.Errorf("expected generated return ID")
		}
		if ret.Status != store.ReturnStatusPending {
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
		if !errors.Is(err, order.ErrUnauthorizedReturn) {
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
		ret, err := s.CreateReturnRequest(ctx, "buyer_1", "ord_paid_online", "Đổi trả hàng", 200000)
		if err != nil {
			t.Fatalf("create return failed: %v", err)
		}

		// Pending -> Approved
		approved, err := s.UpdateReturnStatus(ctx, ret.ID, store.ReturnStatusApproved)
		if err != nil {
			t.Fatalf("Update to Approved failed: %v", err)
		}
		if approved.Status != store.ReturnStatusApproved {
			t.Errorf("expected APPROVED, got %d", approved.Status)
		}

		// Approved -> Refunded
		refunded, err := s.UpdateReturnStatus(ctx, ret.ID, store.ReturnStatusRefunded)
		if err != nil {
			t.Fatalf("Update to Refunded failed: %v", err)
		}
		if refunded.Status != store.ReturnStatusRefunded {
			t.Errorf("expected REFUNDED, got %d", refunded.Status)
		}

		// Refunded -> Approved (Invalid, terminal state)
		_, err = s.UpdateReturnStatus(ctx, ret.ID, store.ReturnStatusApproved)
		if err == nil {
			t.Errorf("expected error updating terminal state, got nil")
		}
	})
}

func TestOrderService_Shipment(t *testing.T) {
	orderRepo := &mockOrderServiceRepo{
		orders: map[string]store.Order{
			"ord_to_ship": {
				ID:          "ord_to_ship",
				BuyerID:     "buyer_1",
				SellerID:    "seller_1",
				TotalAmount: 300000,
				Status:      store.OrderStatusPaid,
			},
		},
	}
	shipmentRepo := fake.NewInMemoryShipmentRepository()
	s := newSvc(t, orderRepo, nil, nil, shipmentRepo, nil, nil, nil)
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
		if ord.Status != store.OrderStatusShipped {
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

// The shipment outbox builder receives the order's buyer and seller, so the
// OrderShipped event can name the user to notify.
func TestOrderService_CreateShipmentCarriesOrderParties(t *testing.T) {
	outbox := newPGOutbox(t.Context(), t)
	orderRepo := outbox.orders()
	if _, err := orderRepo.CreateOrder(context.Background(), store.Order{ID: "ord_parties", BuyerID: "buyer_9", SellerID: "seller_9", Status: store.OrderStatusPaid}); err != nil {
		t.Fatal(err)
	}
	var seen store.Shipment
	shipmentRepo := outbox.shipments(
		store.WithShipmentOutbox(func(sh store.Shipment) (outboxsdk.Row, error) {
			seen = sh
			return outboxsdk.Row{EventID: "e1", AggregateType: "Order", AggregateID: sh.OrderID, EventType: "platform.order.v1.OrderShipped", Payload: []byte("x")}, nil
		}),
	)
	s := newSvc(t, orderRepo, nil, nil, shipmentRepo, nil, nil, nil)

	if _, err := s.CreateShipment(context.Background(), "ord_parties", "GHN", "GHN-1"); err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}
	if seen.BuyerID != "buyer_9" || seen.SellerID != "seller_9" {
		t.Errorf("builder saw buyer/seller %q/%q, want buyer_9/seller_9", seen.BuyerID, seen.SellerID)
	}
	if len(outbox.EnqueuedRows(t.Context())) != 1 {
		t.Errorf("expected 1 outbox row, got %d", len(outbox.EnqueuedRows(t.Context())))
	}
}

func TestOrderService_RejectsBuyingOwnListing(t *testing.T) {
	cartRepo := fake.NewInMemoryCartRepository()
	orderRepo := fake.NewInMemoryOrderRepository()
	ctx := context.Background()
	for _, it := range []store.CartItem{
		{UserID: "seller_1", ListingID: "l1", Quantity: 1, UnitPrice: 1000, SellerID: "seller_2"},
		{UserID: "seller_1", ListingID: "l2", Quantity: 1, UnitPrice: 1000, SellerID: "seller_1"},
	} {
		if _, err := cartRepo.AddItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	// nil domain client: the guard must trip before any stock call.
	s := newSvc(t, orderRepo, cartRepo, nil, nil, nil, nil, nil)
	_, err := s.CreateOrdersFromCart(ctx, "seller_1", store.Address{}, nil, 0, "")
	if !errors.Is(err, order.ErrSelfPurchase) {
		t.Fatalf("err = %v, want ErrSelfPurchase", err)
	}
	if got, _ := orderRepo.ListBuyerOrders(ctx, "seller_1", 0); len(got) != 0 {
		t.Fatalf("no order may be created, got %d", len(got))
	}
}

// Stubs so the fake satisfies store.OrderRepository (order-unpaid-expiry reminder pass).
func (m *mockOrderServiceRepo) FindReminderDue(context.Context, time.Time, time.Duration, int) ([]store.Order, error) {
	return nil, nil
}

func (m *mockOrderServiceRepo) MarkReminderSent(context.Context, string, time.Time) (store.Order, error) {
	return store.Order{}, nil
}
