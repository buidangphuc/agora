package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/grpcserver"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

type mockDomainClient struct {
	listings map[string]*listingv1.Listing
	stocks   map[string]int32
}

func (m *mockDomainClient) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	l, ok := m.listings[req.GetId()]
	if !ok {
		return &listingv1.GetListingResponse{Listing: nil}, nil
	}
	return &listingv1.GetListingResponse{Listing: l}, nil
}

func (m *mockDomainClient) ReserveStock(_ context.Context, req *listingv1.ReserveStockRequest, _ ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	key := req.GetListingId()
	if req.GetVariantId() != "" {
		key = req.GetVariantId()
	}
	cur := m.stocks[key]
	if cur < req.GetQuantity() {
		return nil, service.ErrInsufficientStock
	}
	m.stocks[key] -= req.GetQuantity()
	return &listingv1.ReserveStockResponse{Success: true}, nil
}

func (m *mockDomainClient) ReleaseStock(_ context.Context, req *listingv1.ReleaseStockRequest, _ ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	key := req.GetListingId()
	if req.GetVariantId() != "" {
		key = req.GetVariantId()
	}
	m.stocks[key] += req.GetQuantity()
	return &listingv1.ReleaseStockResponse{Success: true}, nil
}

type mockAddressClient struct {
	addresses []*identityv1.Address
}

func (m *mockAddressClient) ListAddresses(_ context.Context, _ *identityv1.ListAddressesRequest, _ ...grpc.CallOption) (*identityv1.ListAddressesResponse, error) {
	return &identityv1.ListAddressesResponse{Addresses: m.addresses}, nil
}

func (m *mockAddressClient) CreateAddress(_ context.Context, _ *identityv1.CreateAddressRequest, _ ...grpc.CallOption) (*identityv1.CreateAddressResponse, error) {
	return &identityv1.CreateAddressResponse{}, nil
}

func (m *mockAddressClient) UpdateAddress(_ context.Context, _ *identityv1.UpdateAddressRequest, _ ...grpc.CallOption) (*identityv1.UpdateAddressResponse, error) {
	return &identityv1.UpdateAddressResponse{}, nil
}

func (m *mockAddressClient) DeleteAddress(_ context.Context, _ *identityv1.DeleteAddressRequest, _ ...grpc.CallOption) (*identityv1.DeleteAddressResponse, error) {
	return &identityv1.DeleteAddressResponse{}, nil
}

func (m *mockAddressClient) SetDefaultAddress(_ context.Context, _ *identityv1.SetDefaultAddressRequest, _ ...grpc.CallOption) (*identityv1.SetDefaultAddressResponse, error) {
	return &identityv1.SetDefaultAddressResponse{}, nil
}

func principalCtx(t *testing.T, userID string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs(
		"x-principal-id", userID,
		"x-principal-type", "user",
		"x-principal-scopes", "order.read,order.write",
	)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func setupServer(t *testing.T) (orderv1.CartServiceClient, orderv1.OrderServiceClient, *mockDomainClient) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Settings{
		Server: config.Server{Host: "localhost", Port: 0},
	}

	cartRepo := repository.NewInMemoryCartRepository()
	orderRepo := repository.NewInMemoryOrderRepository()

	domain := &mockDomainClient{
		listings: map[string]*listingv1.Listing{
			"list-1": {
				Id:       "list-1",
				Title:    "Áo thun cotton",
				Price:    150000,
				SellerId: "seller-1",
				Variants: []*listingv1.Variant{
					{Id: "var-1", Name: "Màu đen - Size L", Price: 160000, Stock: 5},
				},
			},
			"list-2": {
				Id:       "list-2",
				Title:    "Quần jean nam",
				Price:    350000,
				SellerId: "seller-2",
				Stock:    10,
			},
		},
		stocks: map[string]int32{
			"var-1":  5,
			"list-2": 10,
		},
	}

	addrClient := &mockAddressClient{
		addresses: []*identityv1.Address{
			{
				Id:            "addr-1",
				UserId:        "buyer-1",
				RecipientName: "Nguyen Van A",
				Phone:         "0901234567",
				Street:        "123 Nguyen Hue",
				City:          "TP HCM",
				IsDefault:     true,
			},
		},
	}

	returnRepo := repository.NewInMemoryReturnRepository()
	shipmentRepo := repository.NewInMemoryShipmentRepository()

	cartSvc := service.NewCartService(cartRepo, orderRepo, domain, logger)
	orderSvc := service.NewOrderService(orderRepo, cartRepo, returnRepo, shipmentRepo, domain, addrClient, logger)

	cartHandler := handler.NewCartHandler(cartSvc, logger)
	orderHandler := handler.NewOrderHandler(orderSvc, addrClient, logger)

	srv := grpcserver.Build(cfg, cartHandler, orderHandler, nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(srv.GracefulStop)

	go func() {
		_ = srv.Serve(lis)
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return orderv1.NewCartServiceClient(conn), orderv1.NewOrderServiceClient(conn), domain
}

func TestCartAndCheckout_MultiVendor(t *testing.T) {
	cartClient, orderClient, domain := setupServer(t)

	ctx, cancel := principalCtx(t, "buyer-1")
	defer cancel()

	// 1. Add item 1 (seller 1) to cart
	resp1, err := cartClient.AddToCart(ctx, &orderv1.AddToCartRequest{
		ListingId: "list-1",
		VariantId: "var-1",
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("AddToCart item 1: %v", err)
	}
	if len(resp1.GetCart().GetItems()) != 1 {
		t.Fatalf("want 1 cart item, got %d", len(resp1.GetCart().GetItems()))
	}
	if resp1.GetCart().GetSubtotal() != 320000 {
		t.Fatalf("want subtotal 320000, got %d", resp1.GetCart().GetSubtotal())
	}

	// 2. Add item 2 (seller 2) to cart
	resp2, err := cartClient.AddToCart(ctx, &orderv1.AddToCartRequest{
		ListingId: "list-2",
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("AddToCart item 2: %v", err)
	}
	if len(resp2.GetCart().GetItems()) != 2 {
		t.Fatalf("want 2 cart items, got %d", len(resp2.GetCart().GetItems()))
	}
	if resp2.GetCart().GetSubtotal() != 670000 { // 320000 + 350000
		t.Fatalf("want subtotal 670000, got %d", resp2.GetCart().GetSubtotal())
	}

	// 3. Checkout cart -> Should split into 2 orders for seller-1 and seller-2
	checkoutResp, err := orderClient.CreateOrder(ctx, &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatalf("CreateOrder (Checkout): %v", err)
	}

	if len(checkoutResp.GetOrders()) != 2 {
		t.Fatalf("want 2 orders (multi-vendor split), got %d", len(checkoutResp.GetOrders()))
	}

	// Stock should be reserved: var-1 (5 - 2 = 3), list-2 (10 - 1 = 9)
	if domain.stocks["var-1"] != 3 {
		t.Fatalf("want var-1 stock 3, got %d", domain.stocks["var-1"])
	}
	if domain.stocks["list-2"] != 9 {
		t.Fatalf("want list-2 stock 9, got %d", domain.stocks["list-2"])
	}

	// Cart should be empty after checkout
	cartAfter, err := cartClient.GetCart(ctx, &orderv1.GetCartRequest{})
	if err != nil {
		t.Fatalf("GetCart after checkout: %v", err)
	}
	if len(cartAfter.GetCart().GetItems()) != 0 {
		t.Fatalf("expected empty cart after checkout, got %d items", len(cartAfter.GetCart().GetItems()))
	}

	// 4. Cancel seller-1 order -> stock should be released (var-1 back to 3 + 2 = 5)
	var seller1OrderID string
	for _, o := range checkoutResp.GetOrders() {
		if o.GetSellerId() == "seller-1" {
			seller1OrderID = o.GetId()
			break
		}
	}
	if seller1OrderID == "" {
		t.Fatal("could not find order for seller-1")
	}

	_, err = orderClient.CancelOrder(ctx, &orderv1.CancelOrderRequest{
		Id: seller1OrderID,
	})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if domain.stocks["var-1"] != 5 {
		t.Fatalf("want var-1 stock restored to 5, got %d", domain.stocks["var-1"])
	}
}

func TestShipmentAndRMA_E2E(t *testing.T) {
	cartClient, orderClient, _ := setupServer(t)

	buyerCtx, buyerCancel := principalCtx(t, "buyer-1")
	defer buyerCancel()

	// 1. Add item to cart and checkout
	_, err := cartClient.AddToCart(buyerCtx, &orderv1.AddToCartRequest{
		ListingId: "list-2",
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("AddToCart: %v", err)
	}

	checkoutResp, err := orderClient.CreateOrder(buyerCtx, &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if len(checkoutResp.GetOrders()) != 1 {
		t.Fatalf("expected 1 order, got %d", len(checkoutResp.GetOrders()))
	}
	createdOrder := checkoutResp.GetOrders()[0]
	orderID := createdOrder.GetId()
	sellerID := createdOrder.GetSellerId()

	// 2. Seller creates shipment
	sellerCtx, sellerCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer sellerCancel()
	sellerMd := metadata.Pairs(
		"x-principal-id", sellerID,
		"x-principal-type", "seller",
		"x-principal-scopes", "order.read,order.write",
	)
	sellerCtx = metadata.NewOutgoingContext(sellerCtx, sellerMd)

	shipResp, err := orderClient.CreateShipment(sellerCtx, &orderv1.CreateShipmentRequest{
		OrderId:      orderID,
		Carrier:      "SPX",
		TrackingCode: "SPX-VN-TRACK-101",
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}
	if shipResp.GetShipment().GetTrackingCode() != "SPX-VN-TRACK-101" {
		t.Fatalf("unexpected tracking code: %s", shipResp.GetShipment().GetTrackingCode())
	}

	// 3. Buyer gets shipment tracking
	trackResp, err := orderClient.GetShipmentTracking(buyerCtx, &orderv1.GetShipmentTrackingRequest{
		TrackingCode: "SPX-VN-TRACK-101",
	})
	if err != nil {
		t.Fatalf("GetShipmentTracking: %v", err)
	}
	if trackResp.GetShipment().GetOrderId() != orderID {
		t.Fatalf("expected order id %s, got %s", orderID, trackResp.GetShipment().GetOrderId())
	}
	if len(trackResp.GetShipment().GetCheckpoints()) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(trackResp.GetShipment().GetCheckpoints()))
	}

	// 4. Update order to COMPLETED
	_, err = orderClient.UpdateOrderStatus(buyerCtx, &orderv1.UpdateOrderStatusRequest{
		Id:     orderID,
		Status: orderv1.OrderStatus_ORDER_STATUS_COMPLETED,
	})
	if err != nil {
		t.Fatalf("UpdateOrderStatus to COMPLETED: %v", err)
	}

	// 5. Buyer requests return / refund (RMA)
	returnResp, err := orderClient.CreateReturnRequest(buyerCtx, &orderv1.CreateReturnRequestRequest{
		OrderId:      orderID,
		Reason:       "Sản phẩm bị lỗi kỹ thuật",
		RefundAmount: createdOrder.GetTotalAmount(),
	})
	if err != nil {
		t.Fatalf("CreateReturnRequest: %v", err)
	}
	returnID := returnResp.GetReturnRequest().GetId()
	if returnResp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_PENDING {
		t.Fatalf("expected status PENDING, got %v", returnResp.GetReturnRequest().GetStatus())
	}

	// 6. Get return request
	getReturnResp, err := orderClient.GetReturnRequest(buyerCtx, &orderv1.GetReturnRequestRequest{
		Id: returnID,
	})
	if err != nil {
		t.Fatalf("GetReturnRequest: %v", err)
	}
	if getReturnResp.GetReturnRequest().GetReason() != "Sản phẩm bị lỗi kỹ thuật" {
		t.Fatalf("unexpected reason: %s", getReturnResp.GetReturnRequest().GetReason())
	}

	// 7. Seller approves return request
	updateReturnResp, err := orderClient.UpdateReturnStatus(sellerCtx, &orderv1.UpdateReturnStatusRequest{
		Id:     returnID,
		Status: orderv1.ReturnStatus_RETURN_STATUS_APPROVED,
	})
	if err != nil {
		t.Fatalf("UpdateReturnStatus (APPROVED): %v", err)
	}
	if updateReturnResp.GetReturnRequest().GetStatus() != orderv1.ReturnStatus_RETURN_STATUS_APPROVED {
		t.Fatalf("expected APPROVED status, got %v", updateReturnResp.GetReturnRequest().GetStatus())
	}
}
