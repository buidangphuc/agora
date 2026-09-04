package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/featureflags"
	"github.com/buidangphuc/team-order/internal/interceptor"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
)

type OrderHandler struct {
	orderv1.UnimplementedOrderServiceServer

	svc        *service.OrderService
	addrClient identityv1.AddressServiceClient
	flags      featureflags.Evaluator
	logger     *slog.Logger
}

// Option customizes an OrderHandler. Variadic options keep NewOrderHandler
// backward-compatible with existing call sites.
type Option func(*OrderHandler)

// WithFeatureFlags injects the flag evaluator used to gate the checkout
// kill-switch. When omitted, the handler treats checkout as enabled (fail-open).
func WithFeatureFlags(e featureflags.Evaluator) Option {
	return func(h *OrderHandler) { h.flags = e }
}

func NewOrderHandler(svc *service.OrderService, addrClient identityv1.AddressServiceClient, logger *slog.Logger, opts ...Option) *OrderHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &OrderHandler{svc: svc, addrClient: addrClient, logger: logger}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *OrderHandler) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.CreateOrderResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	// Emergency kill-switch (authoritative enforcement point). Evaluate the
	// `checkout-enabled` flag with default TRUE (fail-open): a Flipt outage must
	// never block checkout, only a deliberate OFF toggle does. When off, reject
	// before running the purchase saga.
	if h.flags != nil && !h.flags.BooleanEnabled(ctx, featureflags.FlagCheckoutEnabled, true) {
		h.logger.Warn("checkout blocked by kill-switch", slog.String("buyer_id", principal.GetId()))
		return nil, status.Error(codes.FailedPrecondition, "checkout is temporarily unavailable")
	}

	var shippingAddr repository.Address
	// Look up shipping address from identity service if client provided
	if h.addrClient != nil {
		resp, err := h.addrClient.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
		if err == nil && resp != nil {
			for _, a := range resp.GetAddresses() {
				if req.GetAddressId() != "" && a.GetId() == req.GetAddressId() {
					shippingAddr = toRepoAddress(a)
					break
				}
				if req.GetAddressId() == "" && a.GetIsDefault() {
					shippingAddr = toRepoAddress(a)
					break
				}
			}
			if shippingAddr.ID == "" && len(resp.GetAddresses()) > 0 {
				shippingAddr = toRepoAddress(resp.GetAddresses()[0])
			}
		}
	}

	orders, err := h.svc.CreateOrdersFromCart(ctx, principal.GetId(), shippingAddr, req.GetItemIds(), int32(req.GetPaymentMethod()), req.GetVoucherCode())
	if err != nil {
		if errors.Is(err, service.ErrEmptyCart) {
			return nil, status.Error(codes.FailedPrecondition, "cart is empty")
		}
		if errors.Is(err, service.ErrVoucherRejected) {
			// Invalid/expired voucher: reject the checkout with the promotion-supplied
			// reason (never silently drop the voucher and charge full price).
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		if errors.Is(err, service.ErrInsufficientStock) {
			return nil, status.Errorf(codes.ResourceExhausted, "stock reservation failed: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "create order: %v", err)
	}

	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.CreateOrderResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) CalculateShippingFee(_ context.Context, req *orderv1.CalculateShippingFeeRequest) (*orderv1.CalculateShippingFeeResponse, error) {
	fee, isFree, msg := h.svc.CalculateShippingFee(req.GetCity(), req.GetItemsSubtotal())
	return &orderv1.CalculateShippingFeeResponse{
		ShippingFee:    fee,
		IsFreeShipping: isFree,
		Message:        msg,
	}, nil
}

func (h *OrderHandler) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}
	o, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}
	if principal, ok := interceptor.PrincipalFromContext(ctx); ok && principal != nil && principal.GetId() != "" {
		if o.BuyerID != principal.GetId() && o.SellerID != principal.GetId() {
			return nil, status.Error(codes.PermissionDenied, "cannot view another user's order")
		}
	}
	return &orderv1.GetOrderResponse{Order: toWireOrder(o)}, nil
}

func (h *OrderHandler) ListBuyerOrders(ctx context.Context, req *orderv1.ListBuyerOrdersRequest) (*orderv1.ListBuyerOrdersResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := h.svc.ListBuyerOrders(ctx, principal.GetId(), int32(req.GetStatusFilter()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list buyer orders: %v", err)
	}
	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.ListBuyerOrdersResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) ListSellerOrders(ctx context.Context, req *orderv1.ListSellerOrdersRequest) (*orderv1.ListSellerOrdersResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := h.svc.ListSellerOrders(ctx, principal.GetId(), int32(req.GetStatusFilter()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list seller orders: %v", err)
	}
	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.ListSellerOrdersResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) UpdateOrderStatus(ctx context.Context, req *orderv1.UpdateOrderStatusRequest) (*orderv1.UpdateOrderStatusResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}
	if req.GetStatus() == orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "target status is required")
	}

	existing, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}

	if principal, ok := interceptor.PrincipalFromContext(ctx); ok && principal != nil && principal.GetId() != "" {
		if existing.SellerID != principal.GetId() && existing.BuyerID != principal.GetId() {
			return nil, status.Error(codes.PermissionDenied, "cannot update another user's order")
		}
	}

	updated, err := h.svc.UpdateOrderStatus(ctx, req.GetId(), repository.OrderStatus(req.GetStatus()), req.GetTrackingNumber())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "update order status: %v", err)
	}
	return &orderv1.UpdateOrderStatusResponse{Order: toWireOrder(updated)}, nil
}

func (h *OrderHandler) CancelOrder(ctx context.Context, req *orderv1.CancelOrderRequest) (*orderv1.CancelOrderResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	existing, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}

	if existing.BuyerID != principal.GetId() {
		return nil, status.Error(codes.PermissionDenied, "only buyer can cancel pending order")
	}

	cancelled, err := h.svc.CancelOrder(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cancel order: %v", err)
	}
	return &orderv1.CancelOrderResponse{Order: toWireOrder(cancelled)}, nil
}

func (h *OrderHandler) GetSagaState(ctx context.Context, req *orderv1.GetSagaStateRequest) (*orderv1.GetSagaStateResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order_id is required")
	}
	order, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Errorf(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}
	// Owner-or-admin only: the saga view exposes another buyer's order state.
	if !isAdminOrUser(principal, order.BuyerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's saga state")
	}

	steps := []*orderv1.SagaStep{
		{
			Name:      "1. Khởi tạo Đơn Hàng (Order Created)",
			Status:    "SUCCESS",
			Timestamp: timestamppb.New(order.CreatedAt),
			Detail:    "Đơn hàng được khởi tạo thành công trên Order DB",
		},
		{
			Name:      "2. Khóa Tồn Kho Sản Phẩm (Stock Reserved)",
			Status:    "SUCCESS",
			Timestamp: timestamppb.New(order.CreatedAt.Add(50 * time.Millisecond)),
			Detail:    "Đã gọi gRPC ReserveStock sang team-domain thành công",
		},
	}

	isCompensated := false
	compReason := ""
	currentStep := "4. Đơn Hàng Hoàn Tất"

	if order.Status == repository.OrderStatusCancelled {
		isCompensated = true
		compReason = "Thanh toán thất bại / Người dùng hủy đơn -> Đã tự động hoàn trả tồn kho (Compensating Transaction: ReleaseStock)"
		currentStep = "Đã Hoàn Tác (Compensated)"
		steps = append(steps, &orderv1.SagaStep{
			Name:      "3. Thanh Toán (Payment Charged)",
			Status:    "FAILED",
			Timestamp: timestamppb.New(order.UpdatedAt),
			Detail:    "Giao dịch thanh toán bị từ chối hoặc thử nghiệm thất bại",
		})
		steps = append(steps, &orderv1.SagaStep{
			Name:      "4. Hoàn Tác & Trả Tồn Kho (Compensation Executed)",
			Status:    "COMPENSATED",
			Timestamp: timestamppb.New(order.UpdatedAt.Add(30 * time.Millisecond)),
			Detail:    "Đã tự động gọi ReleaseStock sang team-domain và hủy đơn hàng minh bạch",
		})
	} else {
		steps = append(steps, &orderv1.SagaStep{
			Name:      "3. Thanh Toán (Payment Charged)",
			Status:    "SUCCESS",
			Timestamp: timestamppb.New(order.CreatedAt.Add(120 * time.Millisecond)),
			Detail:    "Thanh toán xác nhận thành công qua team-payment",
		})
		steps = append(steps, &orderv1.SagaStep{
			Name:      "4. Xác Nhận & Giao Vận (Order Confirmed)",
			Status:    "SUCCESS",
			Timestamp: timestamppb.New(order.CreatedAt.Add(200 * time.Millisecond)),
			Detail:    "Đơn hàng sẵn sàng đóng gói và bàn giao đơn vị vận chuyển",
		})
	}

	return &orderv1.GetSagaStateResponse{
		OrderId:            order.ID,
		CurrentStep:        currentStep,
		Steps:              steps,
		IsCompensated:      isCompensated,
		CompensationReason: compReason,
	}, nil
}

func (h *OrderHandler) ForceFailSaga(ctx context.Context, req *orderv1.ForceFailSagaRequest) (*orderv1.ForceFailSagaResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order_id is required")
	}
	// This force-cancels the order and runs compensation (ReleaseStock), so it
	// must carry the same authority as CancelOrder: the order owner or an admin.
	order, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}
	if !isAdminOrUser(principal, order.BuyerID) {
		return nil, status.Error(codes.PermissionDenied, "only the order owner or an admin can force-fail the saga")
	}

	// Trigger compensation cancellation
	_, err = h.svc.CancelOrder(ctx, req.GetOrderId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "force fail cancel order: %v", err)
	}

	sagaState, err := h.GetSagaState(ctx, &orderv1.GetSagaStateRequest{OrderId: req.GetOrderId()})
	if err != nil {
		return nil, err
	}

	return &orderv1.ForceFailSagaResponse{
		Success:   true,
		Message:   "✓ Đã kích hoạt lỗi giả lập và thực thi Compensating Transaction (ReleaseStock) thành công!",
		SagaState: sagaState,
	}, nil
}

// ── RMA (Return & Refund Management) RPCs ──

func (h *OrderHandler) CreateReturnRequest(ctx context.Context, req *orderv1.CreateReturnRequestRequest) (*orderv1.CreateReturnRequestResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	ret, err := h.svc.CreateReturnRequest(ctx, principal.GetId(), req.GetOrderId(), req.GetReason(), req.GetRefundAmount())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if errors.Is(err, service.ErrUnauthorizedReturn) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if errors.Is(err, service.ErrInvalidReturnReason) || errors.Is(err, service.ErrInvalidRefundAmount) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, service.ErrOrderCannotBeReturned) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "create return request: %v", err)
	}

	return &orderv1.CreateReturnRequestResponse{
		ReturnRequest: toWireOrderReturn(ret),
	}, nil
}

func (h *OrderHandler) GetReturnRequest(ctx context.Context, req *orderv1.GetReturnRequestRequest) (*orderv1.GetReturnRequestResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "return id is required")
	}

	ret, err := h.svc.GetReturnRequest(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, status.Errorf(codes.Internal, "get return request: %v", err)
	}

	if !isAdminOrUser(principal, ret.BuyerID, ret.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's return request")
	}

	return &orderv1.GetReturnRequestResponse{
		ReturnRequest: toWireOrderReturn(ret),
	}, nil
}

func (h *OrderHandler) UpdateReturnStatus(ctx context.Context, req *orderv1.UpdateReturnStatusRequest) (*orderv1.UpdateReturnStatusResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "return id is required")
	}
	if req.GetStatus() == orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "target return status is required")
	}

	existing, err := h.svc.GetReturnRequest(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, status.Errorf(codes.Internal, "get return request: %v", err)
	}

	// Only seller or admin can approve/reject/refund return request
	if !isAdminOrUser(principal, existing.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can update return request status")
	}

	updated, err := h.svc.UpdateReturnStatus(ctx, req.GetId(), repository.ReturnStatus(req.GetStatus()))
	if err != nil {
		if errors.Is(err, service.ErrInvalidReturnStatus) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "update return status: %v", err)
	}

	return &orderv1.UpdateReturnStatusResponse{
		ReturnRequest: toWireOrderReturn(updated),
	}, nil
}

// ── Shipment & Logistics Tracking RPCs ──

func (h *OrderHandler) CreateShipment(ctx context.Context, req *orderv1.CreateShipmentRequest) (*orderv1.CreateShipmentResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}

	if !isAdminOrUser(principal, order.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can create shipment")
	}

	shipment, err := h.svc.CreateShipment(ctx, req.GetOrderId(), req.GetCarrier(), req.GetTrackingCode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create shipment: %v", err)
	}

	return &orderv1.CreateShipmentResponse{
		Shipment: toWireShipment(shipment),
	}, nil
}

func (h *OrderHandler) GetShipmentTracking(ctx context.Context, req *orderv1.GetShipmentTrackingRequest) (*orderv1.GetShipmentTrackingResponse, error) {
	if req.GetTrackingCode() == "" && req.GetOrderId() == "" && req.GetShipmentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tracking_code, order_id, or shipment_id is required")
	}

	shipment, err := h.svc.GetShipmentTracking(ctx, req.GetTrackingCode(), req.GetOrderId(), req.GetShipmentId())
	if err != nil {
		if errors.Is(err, repository.ErrShipmentNotFound) {
			return nil, status.Error(codes.NotFound, "shipment not found")
		}
		return nil, status.Errorf(codes.Internal, "get shipment tracking: %v", err)
	}

	return &orderv1.GetShipmentTrackingResponse{
		Shipment: toWireShipment(shipment),
	}, nil
}

func toRepoAddress(a *identityv1.Address) repository.Address {
	if a == nil {
		return repository.Address{}
	}
	return repository.Address{
		ID:            a.GetId(),
		UserID:        a.GetUserId(),
		RecipientName: a.GetRecipientName(),
		Phone:         a.GetPhone(),
		Street:        a.GetStreet(),
		Ward:          a.GetWard(),
		District:      a.GetDistrict(),
		City:          a.GetCity(),
		IsDefault:     a.GetIsDefault(),
	}
}

func toWireOrder(o repository.Order) *orderv1.Order {
	items := make([]*orderv1.OrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, &orderv1.OrderItem{
			Id:          it.ID,
			ListingId:   it.ListingID,
			VariantId:   it.VariantID,
			Title:       it.Title,
			VariantName: it.VariantName,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			ImageUrl:    it.ImageURL,
		})
	}

	return &orderv1.Order{
		Id:            o.ID,
		BuyerId:       o.BuyerID,
		SellerId:      o.SellerID,
		Status:        orderv1.OrderStatus(o.Status),
		TotalAmount:   o.TotalAmount,
		ItemsSubtotal: o.ItemsSubtotal,
		ShippingFee:   o.ShippingFee,
		PaymentMethod: paymentv1.PaymentMethod(o.PaymentMethod),
		Currency:      o.Currency,
		ShippingAddress: &identityv1.Address{
			Id:            o.ShippingAddress.ID,
			UserId:        o.ShippingAddress.UserID,
			RecipientName: o.ShippingAddress.RecipientName,
			Phone:         o.ShippingAddress.Phone,
			Street:        o.ShippingAddress.Street,
			Ward:          o.ShippingAddress.Ward,
			District:      o.ShippingAddress.District,
			City:          o.ShippingAddress.City,
			IsDefault:     o.ShippingAddress.IsDefault,
		},
		Items:          items,
		TrackingNumber: o.TrackingNumber,
		CreatedAt:      timestamppb.New(o.CreatedAt),
		UpdatedAt:      timestamppb.New(o.UpdatedAt),
		VoucherCode:    o.VoucherCode,
		DiscountAmount: o.DiscountAmount,
	}
}

func toWireOrderReturn(r repository.OrderReturn) *orderv1.OrderReturn {
	return &orderv1.OrderReturn{
		Id:           r.ID,
		OrderId:      r.OrderID,
		BuyerId:      r.BuyerID,
		SellerId:     r.SellerID,
		Reason:       r.Reason,
		RefundAmount: r.RefundAmount,
		Status:       orderv1.ReturnStatus(r.Status),
		CreatedAt:    timestamppb.New(r.CreatedAt),
		UpdatedAt:    timestamppb.New(r.UpdatedAt),
	}
}

func toWireShipmentCheckpoint(cp repository.ShipmentCheckpoint) *orderv1.ShipmentCheckpoint {
	return &orderv1.ShipmentCheckpoint{
		Id:          cp.ID,
		ShipmentId:  cp.ShipmentID,
		Timestamp:   timestamppb.New(cp.Timestamp),
		Location:    cp.Location,
		Description: cp.Description,
		CreatedAt:   timestamppb.New(cp.CreatedAt),
	}
}

func toWireShipment(s repository.Shipment) *orderv1.Shipment {
	cps := make([]*orderv1.ShipmentCheckpoint, 0, len(s.Checkpoints))
	for _, cp := range s.Checkpoints {
		cps = append(cps, toWireShipmentCheckpoint(cp))
	}
	return &orderv1.Shipment{
		Id:           s.ID,
		OrderId:      s.OrderID,
		Carrier:      s.Carrier,
		TrackingCode: s.TrackingCode,
		Status:       orderv1.ShipmentStatus(s.Status),
		Checkpoints:  cps,
		CreatedAt:    timestamppb.New(s.CreatedAt),
		UpdatedAt:    timestamppb.New(s.UpdatedAt),
	}
}

func isAdminOrUser(principal *commonv1.Principal, allowedUserIDs ...string) bool {
	if principal == nil {
		return false
	}
	for _, id := range allowedUserIDs {
		if id != "" && principal.GetId() == id {
			return true
		}
	}
	for _, s := range principal.GetScopes() {
		if s == "admin" || s == "order.admin" || s == "all" {
			return true
		}
	}
	return false
}
