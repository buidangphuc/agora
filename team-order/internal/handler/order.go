package handler

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
	// Only a user places an order; checked before the kill-switch and before
	// anything is reserved.
	if principal.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		return nil, status.Error(codes.PermissionDenied, "only a user can place an order")
	}

	// Emergency kill-switch (authoritative enforcement point). Evaluate the
	// `checkout-enabled` flag with default TRUE (fail-open): a Flipt outage must
	// never block checkout, only a deliberate OFF toggle does. When off, reject
	// before running the purchase saga.
	if h.flags != nil && !h.flags.BooleanEnabled(ctx, featureflags.FlagCheckoutEnabled, true) {
		h.logger.Warn("checkout blocked by kill-switch", slog.String("buyer_id", principal.GetId()))
		return nil, status.Error(codes.FailedPrecondition, "checkout is temporarily unavailable")
	}

	// Idempotency-Key (gRPC metadata forwarded by the gateway): validated before
	// anything is reserved. Absent means a fresh checkout every time.
	idemKey, err := idempotencyKeyFromContext(ctx)
	if err != nil {
		return nil, err
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

	orders, err := h.svc.CreateOrdersFromCart(ctx, principal.GetId(), shippingAddr, req.GetItemIds(), int32(req.GetPaymentMethod()), req.GetVoucherCode(), service.WithIdempotencyKey(idemKey))
	if err != nil {
		if errors.Is(err, service.ErrEmptyCart) {
			return nil, status.Error(codes.FailedPrecondition, "cart is empty")
		}
		if errors.Is(err, service.ErrSelfPurchase) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "cannot buy your own listing", err)
		}
		if errors.Is(err, service.ErrVoucherRejected) {
			// Invalid/expired voucher: reject the checkout with the promotion-supplied
			// reason (never silently drop the voucher and charge full price).
			// err carries only the promotion-supplied reason (user-facing by design).
			h.logger.Warn("voucher rejected", slog.String("buyer_id", principal.GetId()), slog.Any("error", err))
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if errors.Is(err, service.ErrInsufficientStock) {
			return nil, clientErr(h.logger, codes.ResourceExhausted, "stock reservation failed: insufficient stock", err)
		}
		if errors.Is(err, service.ErrCheckoutInProgress) {
			// Same key, first attempt still running: retryable, not a failure.
			return nil, clientErr(h.logger, codes.Aborted, "checkout with this idempotency key is in progress; retry", err)
		}
		if errors.Is(err, service.ErrReservationLost) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "item no longer reserved; please retry checkout", err)
		}
		return nil, internalErr(h.logger, "create order", err)
	}

	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.CreateOrderResponse{Orders: wireOrders}, nil
}

// idempotencyKeyFromContext reads, trims and validates the idempotency-key request
// metadata: "" when absent, INVALID_ARGUMENT when present but not 1..255 printable
// ASCII bytes or sent more than once.
func idempotencyKeyFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}
	vals := md.Get(service.IdempotencyKeyMetadata)
	switch len(vals) {
	case 0:
		return "", nil
	case 1:
	default:
		return "", status.Error(codes.InvalidArgument, "multiple idempotency-key values")
	}
	key, err := service.NormalizeIdempotencyKey(vals[0])
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "idempotency-key must be 1-255 printable ASCII characters")
	}
	return key, nil
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
	// A principal is mandatory: payment/engagement call this over gRPC with a
	// service principal (order.read), everyone else is a gateway-forwarded user.
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	o, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, internalErr(h.logger, "get order", err)
	}
	if !canViewOrder(principal, o) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's order")
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
		return nil, internalErr(h.logger, "list buyer orders", err)
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
		return nil, internalErr(h.logger, "list seller orders", err)
	}
	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.ListSellerOrdersResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) UpdateOrderStatus(ctx context.Context, req *orderv1.UpdateOrderStatusRequest) (*orderv1.UpdateOrderStatusResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
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
		return nil, internalErr(h.logger, "get order", err)
	}

	// Actor class (spec order-lifecycle-guards): the seller, or an admin acting as
	// the seller. Anyone else -- the buyer included, who cancels through
	// CancelOrder -- is refused before the order's status is revealed.
	if !isAdminOrUser(principal, existing.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only the seller or an admin can update order status")
	}

	updated, err := h.svc.UpdateOrderStatus(ctx, req.GetId(), service.ActorSeller, repository.OrderStatus(req.GetStatus()), req.GetTrackingNumber())
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, service.ErrActorForbidden):
			return nil, clientErr(h.logger, codes.PermissionDenied, "this status change is not allowed for the seller", err)
		case errors.Is(err, service.ErrInvalidStatus):
			return nil, clientErr(h.logger, codes.FailedPrecondition, "invalid order status transition", err)
		}
		return nil, internalErr(h.logger, "update order status", err)
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
		return nil, internalErr(h.logger, "get order", err)
	}

	if existing.BuyerID != principal.GetId() {
		return nil, status.Error(codes.PermissionDenied, "only buyer can cancel pending order")
	}

	cancelled, err := h.svc.CancelOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, service.ErrInvalidStatus) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "order cannot be cancelled in its current status", err)
		}
		return nil, internalErr(h.logger, "cancel order", err)
	}
	return &orderv1.CancelOrderResponse{Order: toWireOrder(cancelled.Order)}, nil
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
		return nil, internalErr(h.logger, "get order", err)
	}
	// Owner-or-admin only: the saga view exposes another buyer's order state.
	if !isAdminOrUser(principal, order.BuyerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's saga state")
	}

	return h.sagaStateOf(ctx, order)
}

// sagaStateOf renders the persisted saga view of an order (service.SagaView).
func (h *OrderHandler) sagaStateOf(ctx context.Context, order repository.Order) (*orderv1.GetSagaStateResponse, error) {
	view, err := h.svc.SagaView(ctx, order)
	if err != nil {
		return nil, internalErr(h.logger, "get saga state", err)
	}
	steps := make([]*orderv1.SagaStep, 0, len(view.Steps))
	for _, st := range view.Steps {
		step := &orderv1.SagaStep{Name: st.Name, Status: st.Status, Detail: st.Detail}
		if st.At != nil {
			step.Timestamp = timestamppb.New(*st.At)
		}
		steps = append(steps, step)
	}
	return &orderv1.GetSagaStateResponse{
		OrderId:            order.ID,
		CurrentStep:        view.CurrentStep,
		Steps:              steps,
		IsCompensated:      view.IsCompensated,
		CompensationReason: view.CompensationReason,
	}, nil
}

// ForceFailSaga cancels the order through the normal cancel path (claim, stock
// release, voucher release) and returns the persisted saga view. fail_step must
// be empty, "payment" or "shipping" (checked before any write). success is true
// only when every reservation of the order was released; a parked release answers
// success=false saying the release is pending retry.
func (h *OrderHandler) ForceFailSaga(ctx context.Context, req *orderv1.ForceFailSagaRequest) (*orderv1.ForceFailSagaResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	// Admin-only: the order owner no longer qualifies.
	if !isAdminOrUser(principal) {
		return nil, status.Error(codes.PermissionDenied, "only an admin can force-fail the saga")
	}
	if req.GetOrderId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order_id is required")
	}
	step := req.GetFailStep()
	switch step {
	case "", "payment", "shipping":
	default:
		return nil, status.Errorf(codes.InvalidArgument, "fail_step must be empty, \"payment\" or \"shipping\", got %q", step)
	}
	cancelled, err := h.svc.CancelOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, service.ErrInvalidStatus) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "order cannot be cancelled in its current status", err)
		}
		return nil, internalErr(h.logger, "force fail cancel order", err)
	}
	sagaState, err := h.sagaStateOf(ctx, cancelled.Order)
	if err != nil {
		return nil, err
	}
	if step == "" {
		step = "unspecified"
	}
	if cancelled.ReleasePending {
		return &orderv1.ForceFailSagaResponse{
			Success:   false,
			Message:   "Order cancelled (fail_step=" + step + ") but the stock release is pending retry; it will be retried automatically.",
			SagaState: sagaState,
		}, nil
	}
	return &orderv1.ForceFailSagaResponse{
		Success:   true,
		Message:   "Order cancelled (fail_step=" + step + ") and its stock released.",
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
			return nil, clientErr(h.logger, codes.PermissionDenied, "only the buyer can request a return for this order", err)
		}
		if errors.Is(err, service.ErrInvalidReturnReason) {
			return nil, clientErr(h.logger, codes.InvalidArgument, "return reason is required", err)
		}
		if errors.Is(err, service.ErrInvalidRefundAmount) {
			return nil, clientErr(h.logger, codes.InvalidArgument, "invalid refund amount", err)
		}
		if errors.Is(err, service.ErrOrderCannotBeReturned) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "order cannot be returned in its current status", err)
		}
		if errors.Is(err, service.ErrNothingToReturn) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "order has no returnable amount left", err)
		}
		return nil, internalErr(h.logger, "create return request", err)
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
		return nil, internalErr(h.logger, "get return request", err)
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
		return nil, internalErr(h.logger, "get return request", err)
	}

	// Only seller or admin can approve/reject/refund return request
	if !isAdminOrUser(principal, existing.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can update return request status")
	}

	updated, err := h.svc.UpdateReturnStatus(ctx, req.GetId(), repository.ReturnStatus(req.GetStatus()))
	if err != nil {
		if errors.Is(err, service.ErrInvalidReturnStatus) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, "invalid return status transition", err)
		}
		if errors.Is(err, service.ErrNotPaidOnline) {
			return nil, clientErr(h.logger, codes.FailedPrecondition, service.ErrNotPaidOnline.Error(), err)
		}
		if errors.Is(err, repository.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, internalErr(h.logger, "update return status", err)
	}

	return &orderv1.UpdateReturnStatusResponse{
		ReturnRequest: toWireOrderReturn(updated),
	}, nil
}

// ListOrderReturns lists an order's returns, newest first, to the order's buyer,
// its seller or an admin; everyone else gets PERMISSION_DENIED.
func (h *OrderHandler) ListOrderReturns(ctx context.Context, req *orderv1.ListOrderReturnsRequest) (*orderv1.ListOrderReturnsResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	o, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, internalErr(h.logger, "get order", err)
	}
	if !isAdminOrUser(principal, o.BuyerID, o.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot list the returns of another user's order")
	}
	returns, err := h.svc.ListOrderReturns(ctx, o.ID)
	if err != nil {
		return nil, internalErr(h.logger, "list order returns", err)
	}
	out := make([]*orderv1.OrderReturn, 0, len(returns))
	for _, r := range returns {
		out = append(out, toWireOrderReturn(r))
	}
	return &orderv1.ListOrderReturnsResponse{Returns: out}, nil
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
		return nil, internalErr(h.logger, "get order", err)
	}

	if !isAdminOrUser(principal, order.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can create shipment")
	}

	shipment, err := h.svc.CreateShipment(ctx, req.GetOrderId(), req.GetCarrier(), req.GetTrackingCode())
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, service.ErrInvalidStatus):
			return nil, clientErr(h.logger, codes.FailedPrecondition, "order cannot be shipped in its current status", err)
		}
		return nil, internalErr(h.logger, "create shipment", err)
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
		return nil, internalErr(h.logger, "get shipment tracking", err)
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

	var paidAt *timestamppb.Timestamp
	if o.PaidAt != nil {
		paidAt = timestamppb.New(*o.PaidAt) // unset = never paid online
	}

	return &orderv1.Order{
		PaidAt:        paidAt,
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

// canViewOrder: the order's buyer or seller, an admin, or a SERVICE principal
// holding order.read.
func canViewOrder(p *commonv1.Principal, o repository.Order) bool {
	if isAdminOrUser(p, o.BuyerID, o.SellerID) {
		return true
	}
	if p.GetType() == commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE {
		for _, s := range p.GetScopes() {
			if s == "order.read" {
				return true
			}
		}
	}
	return false
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
		if s == "admin" || s == "order.admin" {
			return true
		}
	}
	return false
}
