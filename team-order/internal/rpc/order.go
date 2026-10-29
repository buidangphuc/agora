package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

const flagCheckoutEnabled = "checkout-enabled"

type OrderHandler struct {
	orderv1.UnimplementedOrderServiceServer

	svc        *order.OrderService
	addrClient identityv1.AddressServiceClient
	flags      flags.Evaluator
	gate       *auditx.Gate
	logger     *slog.Logger
	errs       grpcx.ErrorTable
}

func NewOrderHandler(svc *order.OrderService, addrClient identityv1.AddressServiceClient, ev flags.Evaluator, gate *auditx.Gate, logger *slog.Logger) (*OrderHandler, error) {
	switch {
	case svc == nil:
		return nil, errors.New("rpc: order service is required")
	case addrClient == nil:
		return nil, errors.New("rpc: address client is required")
	case ev == nil:
		return nil, errors.New("rpc: flag evaluator is required")
	case gate == nil:
		return nil, errors.New("rpc: audit gate is required")
	case logger == nil:
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := newErrorTable()
	if err != nil {
		return nil, err
	}
	return &OrderHandler{svc: svc, addrClient: addrClient, flags: ev, gate: gate, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, cart *CartHandler, h *OrderHandler) {
	orderv1.RegisterCartServiceServer(srv, cart)
	orderv1.RegisterOrderServiceServer(srv, h)
}

func newErrorTable() (grpcx.ErrorTable, error) {
	return grpcx.NewErrorTable(
		grpcx.Rule{Target: store.ErrOrderNotFound, Code: codes.NotFound, Message: "order not found"},
		grpcx.Rule{Target: store.ErrReturnNotFound, Code: codes.NotFound, Message: "return request not found"},
		grpcx.Rule{Target: store.ErrShipmentNotFound, Code: codes.NotFound, Message: "shipment not found"},
	)
}

func (h *OrderHandler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

// audited runs action behind the audit gate when admin is true.
func (h *OrderHandler) audited(ctx context.Context, admin bool, ev auditx.Event, action func(context.Context) error) error {
	if !admin {
		return action(ctx)
	}
	return h.gate.Do(ctx, ev, action)
}

// isAdminPath: the caller acts through the order.admin scope (owner or not).
func isAdminPath(p auth.Principal) bool { return p.HasScope(scopeOrderAdmin) }

func (h *OrderHandler) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.CreateOrderResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	// Only a user places an order; checked before the kill-switch and before
	// anything is reserved.
	if principal.Type != auth.TypeUser {
		return nil, status.Error(codes.PermissionDenied, "only a user can place an order")
	}

	if err := validateVouchers(req.GetVoucherCode(), req.GetPlatformVoucherCode(), req.GetShopVouchers(), true); err != nil {
		return nil, err
	}

	// Emergency kill-switch (authoritative enforcement point). Evaluate the
	// `checkout-enabled` flag with default TRUE (fail-open): a Flipt outage must
	// never block checkout, only a deliberate OFF toggle does. When off, reject
	// before running the purchase saga.
	if !h.flags.BooleanEnabled(ctx, flagCheckoutEnabled, true) {
		h.logger.WarnContext(ctx, "checkout blocked by kill-switch")
		return nil, status.Error(codes.FailedPrecondition, "checkout is temporarily unavailable")
	}

	// Idempotency-Key (gRPC metadata forwarded by the gateway): validated before
	// anything is reserved. Absent means a fresh checkout every time.
	idemKey, err := idempotencyKeyFromContext(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := h.addrClient.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
	if err != nil || resp == nil {
		return nil, status.Error(codes.Unavailable, "address service unavailable; please retry")
	}
	var picked *identityv1.Address
	for _, a := range resp.GetAddresses() {
		if (req.GetAddressId() != "" && a.GetId() == req.GetAddressId()) || (req.GetAddressId() == "" && a.GetIsDefault()) {
			picked = a
			break
		}
	}
	if picked == nil || strings.TrimSpace(picked.GetCity()) == "" {
		return nil, status.Error(codes.FailedPrecondition, "address not found")
	}
	if c := req.GetConfirmedAddress(); c != nil && !sameDeliveryAddress(c, picked) {
		h.logger.WarnContext(ctx, "checkout refused: address changed", "address_id", picked.GetId())
		return nil, status.Error(codes.FailedPrecondition, "address changed")
	}
	shippingAddr := toRepoAddress(picked)

	orders, err := h.svc.CreateOrdersFromCart(ctx, principal.ID, shippingAddr, req.GetItemIds(), int32(req.GetPaymentMethod()), req.GetVoucherCode(), order.WithIdempotencyKey(idemKey), order.WithVouchers(req.GetShopVouchers(), req.GetPlatformVoucherCode()))
	if err != nil {
		if errors.Is(err, order.ErrEmptyCart) {
			return nil, status.Error(codes.FailedPrecondition, "cart is empty")
		}
		if errors.Is(err, order.ErrSelfPurchase) {
			return nil, status.Error(codes.FailedPrecondition, "cannot buy your own listing")
		}
		if errors.Is(err, order.ErrVoucherRejected) {
			// Invalid/expired voucher: reject the checkout with the promotion-supplied
			// reason (never silently drop the voucher and charge full price).
			// err carries only the promotion-supplied reason (user-facing by design).
			h.logger.WarnContext(ctx, "voucher rejected", "error", err)
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if errors.Is(err, order.ErrPriceChanged) {
			return nil, status.Error(codes.FailedPrecondition, "price changed")
		}
		if errors.Is(err, order.ErrPromotionUnavailable) {
			return nil, status.Error(codes.Unavailable, "promotion service unavailable; please retry")
		}
		if errors.Is(err, order.ErrInsufficientStock) {
			return nil, status.Error(codes.ResourceExhausted, "stock reservation failed: insufficient stock")
		}
		if errors.Is(err, order.ErrCheckoutInProgress) {
			// Same key, first attempt still running: retryable, not a failure.
			return nil, status.Error(codes.Aborted, "checkout with this idempotency key is in progress; retry")
		}
		if errors.Is(err, order.ErrReservationLost) {
			return nil, status.Error(codes.FailedPrecondition, "item no longer reserved; please retry checkout")
		}
		return nil, h.fail(ctx, err)
	}

	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.CreateOrderResponse{Orders: wireOrders}, nil
}

func validateVouchers(legacy, platform string, shop []*orderv1.ShopVoucher, create bool) error {
	if create && legacy != "" && platform != "" {
		return status.Error(codes.InvalidArgument, "voucher_code and platform_voucher_code cannot be combined")
	}
	if legacy == "" {
		legacy = platform
	}
	seenSeller := make(map[string]struct{}, len(shop))
	seenCode := make(map[string]struct{}, len(shop)+1)
	if legacy != "" {
		seenCode[legacy] = struct{}{}
	}
	for _, sv := range shop {
		if _, dup := seenSeller[sv.GetSellerId()]; dup {
			return status.Error(codes.InvalidArgument, "only one shop voucher per seller")
		}
		seenSeller[sv.GetSellerId()] = struct{}{}
		if _, dup := seenCode[sv.GetCode()]; dup {
			return status.Error(codes.InvalidArgument, "a voucher code can be used only once")
		}
		seenCode[sv.GetCode()] = struct{}{}
	}
	return nil
}

// PreviewCheckout quotes a checkout without reserving anything. USER only.
func (h *OrderHandler) PreviewCheckout(ctx context.Context, req *orderv1.PreviewCheckoutRequest) (*orderv1.PreviewCheckoutResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if principal.Type != auth.TypeUser {
		return nil, status.Error(codes.PermissionDenied, "only a user can preview a checkout")
	}
	if err := validateVouchers("", req.GetPlatformVoucherCode(), req.GetShopVouchers(), false); err != nil {
		return nil, err
	}
	if req.GetAddressId() != "" {
		al, aerr := h.addrClient.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
		if aerr != nil || al == nil {
			return nil, status.Error(codes.Unavailable, "address service unavailable; please retry")
		}
		found := false
		for _, a := range al.GetAddresses() {
			if a.GetId() == req.GetAddressId() && strings.TrimSpace(a.GetCity()) != "" {
				found = true
				break
			}
		}
		if !found {
			return nil, status.Error(codes.FailedPrecondition, "address not found")
		}
	}
	resp, err := h.svc.PreviewCheckout(ctx, principal.ID, req.GetAddressId(), req.GetItemIds(),
		order.WithVouchers(req.GetShopVouchers(), req.GetPlatformVoucherCode()))
	if err != nil {
		switch {
		case errors.Is(err, order.ErrEmptyCart):
			return nil, status.Error(codes.FailedPrecondition, "cart is empty")
		case errors.Is(err, order.ErrAddressNotFound):
			return nil, status.Error(codes.FailedPrecondition, "address not found")
		case errors.Is(err, order.ErrSelfPurchase):
			return nil, status.Error(codes.FailedPrecondition, "cannot buy your own listing")
		case errors.Is(err, order.ErrPriceChanged):
			return nil, status.Error(codes.FailedPrecondition, "price changed")
		case errors.Is(err, order.ErrVoucherRejected):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		case errors.Is(err, order.ErrAddressUnavailable):
			return nil, status.Error(codes.Unavailable, "address service unavailable; please retry")
		case errors.Is(err, order.ErrPromotionUnavailable):
			return nil, status.Error(codes.Unavailable, "promotion service unavailable; please retry")
		}
		return nil, h.fail(ctx, err)
	}
	return resp, nil
}

// idempotencyKeyFromContext reads, trims and validates the idempotency-key request
// metadata: "" when absent, INVALID_ARGUMENT when present but not 1..255 printable
// ASCII bytes or sent more than once.
func idempotencyKeyFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}
	vals := md.Get(order.IdempotencyKeyMetadata)
	switch len(vals) {
	case 0:
		return "", nil
	case 1:
	default:
		return "", status.Error(codes.InvalidArgument, "multiple idempotency-key values")
	}
	key, err := order.NormalizeIdempotencyKey(vals[0])
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "idempotency-key must be 1-255 printable ASCII characters")
	}
	return key, nil
}

func (h *OrderHandler) CalculateShippingFee(_ context.Context, req *orderv1.CalculateShippingFeeRequest) (*orderv1.CalculateShippingFeeResponse, error) {
	fee, isFree, msg, days, at := h.svc.CalculateShippingFee(req.GetCity(), req.GetItemsSubtotal())
	var etaAt *timestamppb.Timestamp
	if strings.TrimSpace(req.GetCity()) == "" {
		days = 0
	} else {
		etaAt = timestamppb.New(at)
	}
	return &orderv1.CalculateShippingFeeResponse{
		ShippingFee:    fee,
		IsFreeShipping: isFree,
		Message:        msg,

		EstimatedDeliveryDays: days,
		EstimatedDeliveryAt:   etaAt,
	}, nil
}

func (h *OrderHandler) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}
	// A principal is mandatory: payment/engagement call this over gRPC with a
	// service principal (order.read), everyone else is a gateway-forwarded user.
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	o, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}
	if !canViewOrder(principal, o) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's order")
	}
	return &orderv1.GetOrderResponse{Order: toWireOrder(o)}, nil
}

func (h *OrderHandler) ListBuyerOrders(ctx context.Context, req *orderv1.ListBuyerOrdersRequest) (*orderv1.ListBuyerOrdersResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := h.svc.ListBuyerOrders(ctx, principal.ID, int32(req.GetStatusFilter()))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.ListBuyerOrdersResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) ListSellerOrders(ctx context.Context, req *orderv1.ListSellerOrdersRequest) (*orderv1.ListSellerOrdersResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := h.svc.ListSellerOrders(ctx, principal.ID, int32(req.GetStatusFilter()))
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	wireOrders := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		wireOrders = append(wireOrders, toWireOrder(o))
	}
	return &orderv1.ListSellerOrdersResponse{Orders: wireOrders}, nil
}

func (h *OrderHandler) UpdateOrderStatus(ctx context.Context, req *orderv1.UpdateOrderStatusRequest) (*orderv1.UpdateOrderStatusResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
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
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}

	// Actor class (spec order-lifecycle-guards): the seller, or an admin acting as
	// the seller. Anyone else -- the buyer included, who cancels through
	// CancelOrder -- is refused before the order's status is revealed.
	if !isAdminOrUser(principal, existing.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only the seller or an admin can update order status")
	}

	var updated store.Order
	err = h.audited(ctx, isAdminPath(principal), auditx.Event{
		ActorID: principal.ID, Action: "order.admin_status_update", TargetType: "order", TargetID: existing.ID,
		Metadata: map[string]string{"seller_id": existing.SellerID, "to_status": req.GetStatus().String()},
	}, func(ctx context.Context) error {
		var uerr error
		updated, uerr = h.svc.UpdateOrderStatus(ctx, req.GetId(), order.ActorSeller, store.OrderStatus(req.GetStatus()), req.GetTrackingNumber())
		return uerr
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, order.ErrActorForbidden):
			return nil, status.Error(codes.PermissionDenied, "this status change is not allowed for the seller")
		case errors.Is(err, order.ErrInvalidStatus):
			return nil, status.Error(codes.FailedPrecondition, "invalid order status transition")
		}
		return nil, h.fail(ctx, err)
	}
	return &orderv1.UpdateOrderStatusResponse{Order: toWireOrder(updated)}, nil
}

// ConfirmReceipt lets the order's buyer (only) complete a Shipped order.
func (h *OrderHandler) ConfirmReceipt(ctx context.Context, req *orderv1.ConfirmReceiptRequest) (*orderv1.ConfirmReceiptResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}
	existing, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}
	if existing.BuyerID != principal.ID {
		return nil, status.Error(codes.PermissionDenied, "only the buyer can confirm receipt")
	}
	done, err := h.svc.ConfirmReceipt(ctx, req.GetId())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, order.ErrInvalidStatus):
			return nil, status.Error(codes.FailedPrecondition, "order is not shipped")
		}
		return nil, h.fail(ctx, err)
	}
	return &orderv1.ConfirmReceiptResponse{Order: toWireOrder(done)}, nil
}

func (h *OrderHandler) CancelOrder(ctx context.Context, req *orderv1.CancelOrderRequest) (*orderv1.CancelOrderResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	existing, err := h.svc.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}

	if existing.BuyerID != principal.ID {
		return nil, status.Error(codes.PermissionDenied, "only buyer can cancel pending order")
	}

	// PAYMENT_EXPIRED and ADMIN_FORCE_FAIL belong to the system and admins.
	if rc := req.GetReasonCode(); rc == orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED || rc == orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL {
		return nil, status.Error(codes.PermissionDenied, "reason code is reserved for system use")
	}

	cancelled, err := h.svc.CancelOrderWithReason(ctx, req.GetId(), req.GetReasonCode())
	if err != nil {
		if errors.Is(err, order.ErrInvalidStatus) {
			return nil, status.Error(codes.FailedPrecondition, "order cannot be cancelled in its current status")
		}
		return nil, h.fail(ctx, err)
	}
	return &orderv1.CancelOrderResponse{Order: toWireOrder(cancelled.Order)}, nil
}

func (h *OrderHandler) GetSagaState(ctx context.Context, req *orderv1.GetSagaStateRequest) (*orderv1.GetSagaStateResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order_id is required")
	}
	ord, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Errorf(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}
	// Owner-or-admin only: the saga view exposes another buyer's order state.
	if !isAdminOrUser(principal, ord.BuyerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's saga state")
	}

	return h.sagaStateOf(ctx, ord)
}

// sagaStateOf renders the persisted saga view of an order (order.SagaView).
func (h *OrderHandler) sagaStateOf(ctx context.Context, ord store.Order) (*orderv1.GetSagaStateResponse, error) {
	view, err := h.svc.SagaView(ctx, ord)
	if err != nil {
		return nil, h.fail(ctx, err)
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
		OrderId:            ord.ID,
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
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	// Admin-only: the order owner no longer qualifies. Both the generic admin
	// marker and the order-domain scope are required (authz-residuals-2).
	if !principal.HasScope(scopeAdmin) || !principal.HasScope(scopeOrderAdmin) {
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
	if _, err := h.svc.GetOrder(ctx, req.GetOrderId()); err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}
	auditStep := step
	if auditStep == "" {
		auditStep = "unspecified"
	}
	var cancelled order.CancelResult
	err = h.gate.Do(ctx, auditx.Event{
		ActorID: principal.ID, Action: "order.force_fail", TargetType: "order", TargetID: req.GetOrderId(),
		Metadata: map[string]string{"fail_step": auditStep},
	}, func(ctx context.Context) error {
		var cerr error
		cancelled, cerr = h.svc.CancelOrderWithReason(ctx, req.GetOrderId(), orderv1.CancelReason_CANCEL_REASON_ADMIN_FORCE_FAIL)
		return cerr
	})
	if err != nil {
		if errors.Is(err, order.ErrInvalidStatus) {
			return nil, status.Error(codes.FailedPrecondition, "order cannot be cancelled in its current status")
		}
		return nil, h.fail(ctx, err)
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
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	ret, err := h.svc.CreateReturnRequest(ctx, principal.ID, req.GetOrderId(), req.GetReason(), req.GetRefundAmount())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		if errors.Is(err, order.ErrUnauthorizedReturn) {
			return nil, status.Error(codes.PermissionDenied, "only the buyer can request a return for this order")
		}
		if errors.Is(err, order.ErrInvalidReturnReason) {
			return nil, status.Error(codes.InvalidArgument, "return reason is required")
		}
		if errors.Is(err, order.ErrInvalidRefundAmount) {
			return nil, status.Error(codes.InvalidArgument, "invalid refund amount")
		}
		if errors.Is(err, order.ErrReturnWindowClosed) {
			return nil, status.Error(codes.FailedPrecondition, "return window closed")
		}
		if errors.Is(err, order.ErrOrderCannotBeReturned) {
			return nil, status.Error(codes.FailedPrecondition, "order cannot be returned in its current status")
		}
		if errors.Is(err, order.ErrNothingToReturn) {
			return nil, status.Error(codes.FailedPrecondition, "order has no returnable amount left")
		}
		return nil, h.fail(ctx, err)
	}

	return &orderv1.CreateReturnRequestResponse{
		ReturnRequest: h.wireReturn(ctx, ret),
	}, nil
}

func (h *OrderHandler) GetReturnRequest(ctx context.Context, req *orderv1.GetReturnRequestRequest) (*orderv1.GetReturnRequestResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "return id is required")
	}

	ret, err := h.svc.GetReturnRequest(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, store.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, h.fail(ctx, err)
	}

	if !isAdminOrUser(principal, ret.BuyerID, ret.SellerID) && !isServiceWithScope(principal, "order.read") {
		return nil, status.Error(codes.PermissionDenied, "cannot view another user's return request")
	}

	return &orderv1.GetReturnRequestResponse{
		ReturnRequest: h.wireReturn(ctx, ret),
	}, nil
}

func (h *OrderHandler) UpdateReturnStatus(ctx context.Context, req *orderv1.UpdateReturnStatusRequest) (*orderv1.UpdateReturnStatusResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
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
		if errors.Is(err, store.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, h.fail(ctx, err)
	}

	// Only seller or admin can approve/reject/refund return request
	if !isAdminOrUser(principal, existing.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can update return request status")
	}

	var updated store.OrderReturn
	err = h.audited(ctx, isAdminPath(principal), auditx.Event{
		ActorID: principal.ID, Action: "return.admin_status_update", TargetType: "return_request", TargetID: existing.ID,
		Metadata: map[string]string{"order_id": existing.OrderID, "to_status": req.GetStatus().String()},
	}, func(ctx context.Context) error {
		var uerr error
		updated, uerr = h.svc.UpdateReturnStatus(ctx, req.GetId(), store.ReturnStatus(req.GetStatus()))
		return uerr
	})
	if err != nil {
		if errors.Is(err, order.ErrInvalidReturnStatus) {
			return nil, status.Error(codes.FailedPrecondition, "invalid return status transition")
		}
		if errors.Is(err, order.ErrNotPaidOnline) {
			return nil, status.Error(codes.FailedPrecondition, order.ErrNotPaidOnline.Error())
		}
		if errors.Is(err, store.ErrReturnNotFound) {
			return nil, status.Error(codes.NotFound, "return request not found")
		}
		return nil, h.fail(ctx, err)
	}

	return &orderv1.UpdateReturnStatusResponse{
		ReturnRequest: h.wireReturn(ctx, updated),
	}, nil
}

// ListOrderReturns lists an order's returns, newest first, to the order's buyer,
// its seller or an admin; everyone else gets PERMISSION_DENIED.
func (h *OrderHandler) ListOrderReturns(ctx context.Context, req *orderv1.ListOrderReturnsRequest) (*orderv1.ListOrderReturnsResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	o, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}
	if !isAdminOrUser(principal, o.BuyerID, o.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "cannot list the returns of another user's order")
	}
	returns, err := h.svc.ListOrderReturns(ctx, o.ID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*orderv1.OrderReturn, 0, len(returns))
	for _, r := range returns {
		w := toWireOrderReturn(r)
		w.CanEscalate = h.svc.CanEscalate(r, o)
		out = append(out, w)
	}
	return &orderv1.ListOrderReturnsResponse{Returns: out}, nil
}

// ── Shipment & Logistics Tracking RPCs ──

func (h *OrderHandler) CreateShipment(ctx context.Context, req *orderv1.CreateShipmentRequest) (*orderv1.CreateShipmentResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	ord, err := h.svc.GetOrder(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, store.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, h.fail(ctx, err)
	}

	if !isAdminOrUser(principal, ord.SellerID) {
		return nil, status.Error(codes.PermissionDenied, "only seller or admin can create shipment")
	}

	carrier := req.GetCarrier()
	if carrier == "" {
		carrier = "SPX"
	}
	if r := []rune(carrier); len(r) > 64 {
		carrier = string(r[:64])
	}
	var shipment store.Shipment
	err = h.audited(ctx, isAdminPath(principal), auditx.Event{
		ActorID: principal.ID, Action: "shipment.admin_create", TargetType: "order", TargetID: ord.ID,
		Metadata: map[string]string{"carrier": carrier},
	}, func(ctx context.Context) error {
		var serr error
		shipment, serr = h.svc.CreateShipment(ctx, req.GetOrderId(), req.GetCarrier(), req.GetTrackingCode())
		return serr
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, order.ErrInvalidStatus):
			return nil, status.Error(codes.FailedPrecondition, "order cannot be shipped in its current status")
		}
		return nil, h.fail(ctx, err)
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
		if errors.Is(err, store.ErrShipmentNotFound) {
			return nil, status.Error(codes.NotFound, "shipment not found")
		}
		return nil, h.fail(ctx, err)
	}

	return &orderv1.GetShipmentTrackingResponse{
		Shipment: toWireShipment(shipment),
	}, nil
}

func sameDeliveryAddress(a, b *identityv1.Address) bool {
	return a.GetRecipientName() == b.GetRecipientName() &&
		a.GetPhone() == b.GetPhone() &&
		a.GetStreet() == b.GetStreet() &&
		a.GetWard() == b.GetWard() &&
		a.GetDistrict() == b.GetDistrict() &&
		a.GetCity() == b.GetCity()
}

func toRepoAddress(a *identityv1.Address) store.Address {
	if a == nil {
		return store.Address{}
	}
	return store.Address{
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

func toWireOrder(o store.Order) *orderv1.Order {
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

	var due *timestamppb.Timestamp
	if o.PaymentDueAt != nil {
		due = timestamppb.New(*o.PaymentDueAt)
	}

	ts := func(t *time.Time) *timestamppb.Timestamp {
		if t == nil {
			return nil
		}
		return timestamppb.New(*t)
	}

	return &orderv1.Order{
		ShipBy:             ts(o.ShipBy),
		ShippedAt:          ts(o.ShippedAt),
		CompletedAt:        ts(o.CompletedAt),
		ReturnWindowEndsAt: ts(o.ReturnWindowEndsAt),
		AutoCompleteAt:     ts(o.AutoCompleteAt),
		PaidAt:             paidAt,
		PaymentDueAt:       due,
		Id:                 o.ID,
		BuyerId:            o.BuyerID,
		SellerId:           o.SellerID,
		Status:             orderv1.OrderStatus(o.Status),
		CancelReason:       orderv1.CancelReason(o.CancelReason),
		TotalAmount:        o.TotalAmount,
		ItemsSubtotal:      o.ItemsSubtotal,
		ShippingFee:        o.ShippingFee,
		PaymentMethod:      paymentv1.PaymentMethod(o.PaymentMethod),
		Currency:           o.Currency,
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
		Items:                  items,
		TrackingNumber:         o.TrackingNumber,
		CreatedAt:              timestamppb.New(o.CreatedAt),
		UpdatedAt:              timestamppb.New(o.UpdatedAt),
		VoucherCode:            o.VoucherCode,
		DiscountAmount:         o.DiscountAmount,
		CheckoutId:             o.CheckoutID,
		PlatformVoucherCode:    o.PlatformVoucherCode,
		PlatformDiscountAmount: o.PlatformDiscountAmount,
	}
}

// wireReturn converts a return and computes can_escalate from its order; a
// failed order lookup leaves can_escalate false.
func (h *OrderHandler) wireReturn(ctx context.Context, r store.OrderReturn) *orderv1.OrderReturn {
	w := toWireOrderReturn(r)
	if r.Status != store.ReturnStatusRejected {
		return w
	}
	if o, err := h.svc.GetOrder(ctx, r.OrderID); err == nil {
		w.CanEscalate = h.svc.CanEscalate(r, o)
	}
	return w
}

func toWireOrderReturn(r store.OrderReturn) *orderv1.OrderReturn {
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

func toWireShipmentCheckpoint(cp store.ShipmentCheckpoint) *orderv1.ShipmentCheckpoint {
	return &orderv1.ShipmentCheckpoint{
		Id:          cp.ID,
		ShipmentId:  cp.ShipmentID,
		Timestamp:   timestamppb.New(cp.Timestamp),
		Location:    cp.Location,
		Description: cp.Description,
		CreatedAt:   timestamppb.New(cp.CreatedAt),
	}
}

func toWireShipment(s store.Shipment) *orderv1.Shipment {
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
func canViewOrder(p auth.Principal, o store.Order) bool {
	if isAdminOrUser(p, o.BuyerID, o.SellerID) {
		return true
	}
	return isServiceWithScope(p, "order.read")
}

func isServiceWithScope(p auth.Principal, scope string) bool {
	return p.Type == auth.TypeService && p.HasScope(scope)
}

const (
	scopeAdmin      = "admin"
	scopeOrderAdmin = "order.admin"
)

// isAdminOrUser reports whether the principal is one of allowedUserIDs or an
// order admin. The admin override is the order.admin scope only: the generic
// `admin` marker does not open orders (authz-residuals-2).
func isAdminOrUser(principal auth.Principal, allowedUserIDs ...string) bool {
	for _, id := range allowedUserIDs {
		if id != "" && principal.ID == id {
			return true
		}
	}
	return principal.HasScope(scopeOrderAdmin)
}
