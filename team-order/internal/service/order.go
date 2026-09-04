package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/upstream"
)

var (
	ErrEmptyCart             = errors.New("cart is empty")
	ErrInsufficientStock     = errors.New("insufficient stock for item")
	ErrInvalidStatus         = errors.New("invalid order status transition")
	ErrInvalidReturnReason   = errors.New("return reason is required")
	ErrInvalidRefundAmount   = errors.New("invalid refund amount")
	ErrUnauthorizedReturn    = errors.New("only buyer can request return for this order")
	ErrOrderCannotBeReturned = errors.New("order cannot be returned in current status")
	ErrInvalidReturnStatus   = errors.New("invalid return status transition")
)

type OrderService struct {
	orderRepo    repository.OrderRepository
	cartRepo     repository.CartRepository
	returnRepo   repository.ReturnRepository
	shipmentRepo repository.ShipmentRepository
	sagaRepo     repository.SagaRepository
	domainClient upstream.DomainClient
	addrClient   identityv1.AddressServiceClient
	promo        PromotionClient
	logger       *slog.Logger

	reservationTTL time.Duration
	releaseCfg     releaseRetryConfig
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	cartRepo repository.CartRepository,
	returnRepo repository.ReturnRepository,
	shipmentRepo repository.ShipmentRepository,
	domainClient upstream.DomainClient,
	addrClient identityv1.AddressServiceClient,
	logger *slog.Logger,
	opts ...OrderServiceOption,
) *OrderService {
	if logger == nil {
		logger = slog.Default()
	}
	if returnRepo == nil {
		returnRepo = repository.NewInMemoryReturnRepository()
	}
	if shipmentRepo == nil {
		shipmentRepo = repository.NewInMemoryShipmentRepository()
	}
	s := &OrderService{
		orderRepo:      orderRepo,
		cartRepo:       cartRepo,
		returnRepo:     returnRepo,
		shipmentRepo:   shipmentRepo,
		sagaRepo:       repository.NewInMemorySagaRepository(),
		domainClient:   domainClient,
		addrClient:     addrClient,
		logger:         logger,
		reservationTTL: defaultReservationTTL,
		releaseCfg:     releaseRetryConfig{}.withDefaults(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *OrderService) CalculateShippingFee(city string, itemsSubtotal int64) (int64, bool, string) {
	if itemsSubtotal >= 500000 {
		return 0, true, "Miễn phí vận chuyển cho đơn hàng từ 500.000 VND"
	}
	cityUpper := strings.ToUpper(city)
	if strings.Contains(cityUpper, "HỒ CHÍ MINH") || strings.Contains(cityUpper, "HCM") || strings.Contains(cityUpper, "HÀ NỘI") || strings.Contains(cityUpper, "HN") {
		return 20000, false, "Phí vận chuyển nội thành (20.000 VND)"
	}
	return 35000, false, "Phí vận chuyển toàn quốc (35.000 VND)"
}

func (s *OrderService) CreateOrdersFromCart(
	ctx context.Context,
	buyerID string,
	shippingAddr repository.Address,
	targetItemIDs []string,
	paymentMethod int32,
	voucherCode string,
) ([]repository.Order, error) {
	cartItems, err := s.cartRepo.GetCart(ctx, buyerID)
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}
	if len(cartItems) == 0 {
		return nil, ErrEmptyCart
	}

	// Filter target items if specified
	var itemsToCheckout []repository.CartItem
	if len(targetItemIDs) > 0 {
		idMap := make(map[string]struct{}, len(targetItemIDs))
		for _, id := range targetItemIDs {
			idMap[id] = struct{}{}
		}
		for _, it := range cartItems {
			if _, ok := idMap[it.ID]; ok {
				itemsToCheckout = append(itemsToCheckout, it)
			}
		}
	} else {
		itemsToCheckout = cartItems
	}

	if len(itemsToCheckout) == 0 {
		return nil, ErrEmptyCart
	}

	// Group items by seller_id for multi-vendor orders
	sellerGroups := make(map[string][]repository.CartItem)
	for _, it := range itemsToCheckout {
		sellerID := it.SellerID
		if sellerID == "" {
			sellerID = "unknown_seller"
		}
		sellerGroups[sellerID] = append(sellerGroups[sellerID], it)
	}

	// Persist a durable saga header so reservations are recoverable across a
	// crash/restart (AD3). Compensation always fetches reservation state from this
	// store — never from a request-scoped in-memory slice that a crash would lose.
	saga, err := s.sagaRepo.CreateSaga(ctx, repository.Saga{BuyerID: buyerID})
	if err != nil {
		return nil, fmt.Errorf("create saga: %w", err)
	}
	expiresAt := time.Now().Add(s.reservationTTL)

	// voucherReservationID is the id of the voucher hold placed for this checkout
	// (empty until a voucher is reserved). It equals the order id it discounts so
	// compensation and the settle-time commit can address the same hold with only
	// the order id (see redemption.go). Captured by the compensation closure below.
	var voucherReservationID string

	// failAndCompensate releases the stock held by this saga's un-committed
	// reservations. It reads the durable reservation set (not a cached slice) so
	// COMMITTED rows — those of already-persisted seller-orders — are skipped and
	// their stock is never released (M7). It uses a background context because the
	// request context may already be cancelled/timed out (AD3). Any voucher hold
	// placed for this checkout is released on the same background context.
	failAndCompensate := func(cause error) ([]repository.Order, error) {
		bg := context.Background()
		all, lerr := s.sagaRepo.ListReservationsBySaga(bg, saga.ID)
		if lerr != nil {
			s.logger.ErrorContext(bg, "failed to load reservations for compensation",
				slog.String("saga_id", saga.ID), slog.Any("err", lerr))
		}
		s.compensate(all)
		s.releaseVoucher(bg, voucherReservationID)
		if uerr := s.sagaRepo.UpdateSagaStatus(bg, saga.ID, repository.SagaStatusCompensated); uerr != nil {
			s.logger.ErrorContext(bg, "failed to mark saga compensated",
				slog.String("saga_id", saga.ID), slog.Any("err", uerr))
		}
		return nil, cause
	}

	var createdOrders []repository.Order
	var checkedOutCartItemIDs []string

	// Process one seller-order at a time: reserve its items, persist the order,
	// then COMMIT its reservations. A failure for a later seller therefore only
	// releases the failing seller's un-committed reservations (M7).
	for sellerID, group := range sellerGroups {
		var sellerReservations []repository.Reservation
		var orderItems []repository.OrderItem
		var itemsSubtotal int64

		for _, it := range group {
			resID := ReservationID(buyerID, it) // AD5/M6: stable per (cart_item, attempt)
			res := repository.Reservation{
				ID:        resID,
				SagaID:    saga.ID,
				SellerID:  sellerID,
				BuyerID:   buyerID,
				ListingID: it.ListingID,
				VariantID: it.VariantID,
				Quantity:  it.Quantity,
				Status:    repository.ReservationStatusPending,
				ExpiresAt: expiresAt,
			}
			// Persist reservation intent BEFORE the external effect (AD3).
			if _, cerr := s.sagaRepo.CreateReservation(ctx, res); cerr != nil {
				return failAndCompensate(fmt.Errorf("persist reservation: %w", cerr))
			}

			_, rerr := s.domainClient.ReserveStock(ctx, &listingv1.ReserveStockRequest{
				ListingId:     it.ListingID,
				VariantId:     it.VariantID,
				Quantity:      it.Quantity,
				ReservationId: resID,
			})
			if rerr != nil {
				s.logger.WarnContext(ctx, "stock reservation failed",
					slog.String("listing_id", it.ListingID),
					slog.String("variant_id", it.VariantID),
					slog.Int("qty", int(it.Quantity)),
					slog.Any("err", rerr),
				)
				// This reservation never held stock — mark FAILED so the sweep skips it.
				if uerr := s.sagaRepo.UpdateReservationStatus(ctx, resID, repository.ReservationStatusFailed); uerr != nil {
					s.logger.WarnContext(ctx, "failed to mark reservation failed",
						slog.String("reservation_id", resID), slog.Any("err", uerr))
				}
				return failAndCompensate(fmt.Errorf("%w: %s (%v)", ErrInsufficientStock, it.Title, rerr))
			}
			// Stock is now held; record that durably so the sweep/compensation can
			// reclaim it if we crash before the order persists (fixes SA-C2).
			if uerr := s.sagaRepo.UpdateReservationStatus(ctx, resID, repository.ReservationStatusReserved); uerr != nil {
				s.logger.WarnContext(ctx, "failed to mark reservation reserved",
					slog.String("reservation_id", resID), slog.Any("err", uerr))
			}
			res.Status = repository.ReservationStatusReserved
			sellerReservations = append(sellerReservations, res)

			orderItems = append(orderItems, repository.OrderItem{
				ListingID:   it.ListingID,
				VariantID:   it.VariantID,
				Title:       it.Title,
				VariantName: it.VariantName,
				Quantity:    it.Quantity,
				UnitPrice:   it.UnitPrice,
				ImageURL:    it.ImageURL,
			})
			itemsSubtotal += it.UnitPrice * int64(it.Quantity)
		}

		shippingFee, _, _ := s.CalculateShippingFee(shippingAddr.City, itemsSubtotal)
		totalAmount := itemsSubtotal + shippingFee

		if paymentMethod <= 0 {
			paymentMethod = 1 // default COD
		}

		// Voucher redemption (W1-T2): apply once per checkout, on the first
		// seller-order. The order id is pre-generated and used as the reservation_id
		// (idempotency key) so CommitReservation on PaymentSettled and
		// ReleaseReservation on cancel/compensation address the same hold with just
		// the order id. Empty voucher_code, or no promotion client, leaves this path
		// byte-for-byte unchanged from the pre-voucher behavior. A declined voucher
		// aborts the saga (ErrVoucherRejected → FailedPrecondition), releasing the
		// stock reserved so far — it is never silently dropped.
		orderID := uuid.NewString()
		var discountAmount int64
		var appliedVoucher string
		if voucherCode != "" && s.promo != nil && voucherReservationID == "" {
			discount, verr := s.reserveVoucher(ctx, orderID, voucherCode, buyerID, itemsSubtotal, sellerID)
			if verr != nil {
				return failAndCompensate(verr)
			}
			discountAmount = discount
			appliedVoucher = voucherCode
			voucherReservationID = orderID
			totalAmount = itemsSubtotal + shippingFee - discountAmount
			if totalAmount < 0 {
				totalAmount = 0 // floor at 0: a discount never yields a negative total
			}
		}

		order := repository.Order{
			ID:              orderID,
			BuyerID:         buyerID,
			SellerID:        sellerID,
			Status:          repository.OrderStatusPending,
			TotalAmount:     totalAmount,
			ItemsSubtotal:   itemsSubtotal,
			ShippingFee:     shippingFee,
			PaymentMethod:   paymentMethod,
			Currency:        "VND",
			ShippingAddress: shippingAddr,
			Items:           orderItems,
			VoucherCode:     appliedVoucher,
			DiscountAmount:  discountAmount,
		}

		savedOrder, err := s.orderRepo.CreateOrder(ctx, order)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to persist order", slog.Any("err", err))
			return failAndCompensate(fmt.Errorf("create order: %w", err))
		}

		// Order is durably persisted → COMMIT its reservations (M7): from here their
		// stock is owned by a real order and must never be released.
		for i := range sellerReservations {
			if uerr := s.sagaRepo.CommitReservation(ctx, sellerReservations[i].ID, savedOrder.ID); uerr != nil {
				s.logger.WarnContext(ctx, "failed to commit reservation",
					slog.String("reservation_id", sellerReservations[i].ID), slog.Any("err", uerr))
			}
		}

		createdOrders = append(createdOrders, savedOrder)
		for _, it := range group {
			checkedOutCartItemIDs = append(checkedOutCartItemIDs, it.ID)
		}
	}

	if uerr := s.sagaRepo.UpdateSagaStatus(ctx, saga.ID, repository.SagaStatusCompleted); uerr != nil {
		s.logger.WarnContext(ctx, "failed to mark saga completed",
			slog.String("saga_id", saga.ID), slog.Any("err", uerr))
	}

	// Remove checked-out items from the cart. The orders already exist, so this is
	// best-effort — but a failure must be handled, not silently discarded.
	if err := s.cartRepo.RemoveItems(ctx, buyerID, checkedOutCartItemIDs); err != nil {
		s.logger.ErrorContext(ctx, "failed to remove checked-out items from cart",
			slog.String("buyer_id", buyerID),
			slog.Int("item_count", len(checkedOutCartItemIDs)),
			slog.Any("err", err),
		)
	}

	return createdOrders, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id string) (repository.Order, error) {
	return s.orderRepo.GetOrder(ctx, id)
}

func (s *OrderService) ListBuyerOrders(ctx context.Context, buyerID string, statusFilter int32) ([]repository.Order, error) {
	return s.orderRepo.ListBuyerOrders(ctx, buyerID, statusFilter)
}

func (s *OrderService) ListSellerOrders(ctx context.Context, sellerID string, statusFilter int32) ([]repository.Order, error) {
	return s.orderRepo.ListSellerOrders(ctx, sellerID, statusFilter)
}

func (s *OrderService) UpdateOrderStatus(ctx context.Context, id string, status repository.OrderStatus, trackingNumber string) (repository.Order, error) {
	return s.orderRepo.UpdateOrderStatus(ctx, id, status, trackingNumber)
}

func (s *OrderService) CancelOrder(ctx context.Context, id string) (repository.Order, error) {
	order, err := s.orderRepo.GetOrder(ctx, id)
	if err != nil {
		return repository.Order{}, err
	}

	if order.Status == repository.OrderStatusCancelled || order.Status == repository.OrderStatusCompleted {
		return repository.Order{}, fmt.Errorf("%w: cannot cancel order in status %v", ErrInvalidStatus, order.Status)
	}

	// Release stock for all items in the cancelled order
	for _, it := range order.Items {
		// Idempotent release: a stable cancel-scoped reservation_id so a retried
		// cancel does not double-restore stock (parity with saga compensation, AD5).
		relID := ReservationID(order.BuyerID, repository.CartItem{
			ID: "cancel:" + it.ID, ListingID: it.ListingID, VariantID: it.VariantID, Quantity: it.Quantity,
		})
		_, err := s.domainClient.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{
			ListingId:     it.ListingID,
			VariantId:     it.VariantID,
			Quantity:      it.Quantity,
			ReservationId: relID,
		})
		if err != nil {
			s.logger.WarnContext(ctx, "failed to release stock on cancel",
				slog.String("order_id", id),
				slog.String("listing_id", it.ListingID),
				slog.Any("err", err),
			)
		}
	}

	return s.orderRepo.UpdateOrderStatus(ctx, id, repository.OrderStatusCancelled, "")
}

// ── RMA / Return Management ──

func (s *OrderService) CreateReturnRequest(ctx context.Context, buyerID, orderID, reason string, refundAmount int64) (repository.OrderReturn, error) {
	if strings.TrimSpace(reason) == "" {
		return repository.OrderReturn{}, ErrInvalidReturnReason
	}
	if orderID == "" {
		return repository.OrderReturn{}, repository.ErrOrderNotFound
	}

	order, err := s.orderRepo.GetOrder(ctx, orderID)
	if err != nil {
		return repository.OrderReturn{}, err
	}

	if order.BuyerID != buyerID {
		return repository.OrderReturn{}, ErrUnauthorizedReturn
	}

	if order.Status == repository.OrderStatusCancelled || order.Status == repository.OrderStatusPending {
		return repository.OrderReturn{}, fmt.Errorf("%w: status %v", ErrOrderCannotBeReturned, order.Status)
	}

	if refundAmount <= 0 {
		refundAmount = order.TotalAmount
	} else if refundAmount > order.TotalAmount {
		return repository.OrderReturn{}, fmt.Errorf("%w: refund amount %d exceeds order total %d", ErrInvalidRefundAmount, refundAmount, order.TotalAmount)
	}

	req := repository.OrderReturn{
		OrderID:      orderID,
		BuyerID:      buyerID,
		SellerID:     order.SellerID,
		Reason:       reason,
		RefundAmount: refundAmount,
		Status:       repository.ReturnStatusPending,
	}

	created, err := s.returnRepo.CreateReturn(ctx, req)
	if err != nil {
		return repository.OrderReturn{}, fmt.Errorf("create return in repo: %w", err)
	}

	s.logger.InfoContext(ctx, "created return request",
		slog.String("return_id", created.ID),
		slog.String("order_id", orderID),
		slog.Int64("refund_amount", refundAmount),
	)

	return created, nil
}

func (s *OrderService) GetReturnRequest(ctx context.Context, id string) (repository.OrderReturn, error) {
	return s.returnRepo.GetReturn(ctx, id)
}

func (s *OrderService) UpdateReturnStatus(ctx context.Context, id string, newStatus repository.ReturnStatus) (repository.OrderReturn, error) {
	existing, err := s.returnRepo.GetReturn(ctx, id)
	if err != nil {
		return repository.OrderReturn{}, err
	}

	// Validate status transitions
	switch existing.Status {
	case repository.ReturnStatusPending:
		if newStatus != repository.ReturnStatusApproved && newStatus != repository.ReturnStatusRejected {
			return repository.OrderReturn{}, fmt.Errorf("%w: pending can only transition to approved or rejected", ErrInvalidReturnStatus)
		}
	case repository.ReturnStatusApproved:
		if newStatus != repository.ReturnStatusRefunded && newStatus != repository.ReturnStatusRejected {
			return repository.OrderReturn{}, fmt.Errorf("%w: approved can only transition to refunded or rejected", ErrInvalidReturnStatus)
		}
	case repository.ReturnStatusRejected, repository.ReturnStatusRefunded:
		return repository.OrderReturn{}, fmt.Errorf("%w: return is already in terminal state %v", ErrInvalidReturnStatus, existing.Status)
	}

	updated, err := s.returnRepo.UpdateReturnStatus(ctx, id, newStatus)
	if err != nil {
		return repository.OrderReturn{}, err
	}

	s.logger.InfoContext(ctx, "updated return request status",
		slog.String("return_id", id),
		slog.Int("old_status", int(existing.Status)),
		slog.Int("new_status", int(newStatus)),
	)

	return updated, nil
}

// ── Shipment & Logistics Tracking ──

func (s *OrderService) CreateShipment(ctx context.Context, orderID, carrier, trackingCode string) (repository.Shipment, error) {
	if orderID == "" {
		return repository.Shipment{}, repository.ErrOrderNotFound
	}

	order, err := s.orderRepo.GetOrder(ctx, orderID)
	if err != nil {
		return repository.Shipment{}, err
	}

	if carrier == "" {
		carrier = "SPX"
	}

	if trackingCode == "" {
		orderShort := orderID
		if len(orderShort) > 8 {
			orderShort = orderShort[:8]
		}
		trackingCode = fmt.Sprintf("%s-VN-%s-%d", strings.ToUpper(carrier), strings.ToUpper(orderShort), time.Now().Unix()%1000000)
	}

	now := time.Now()
	initialCheckpoint := repository.ShipmentCheckpoint{
		Timestamp:   now,
		Location:    "Trung tâm phân loại & Bưu cục tiếp nhận",
		Description: fmt.Sprintf("Người bán đã bàn giao kiện hàng cho đơn vị vận chuyển %s", carrier),
		CreatedAt:   now,
	}

	shipment := repository.Shipment{
		OrderID:      orderID,
		Carrier:      carrier,
		TrackingCode: trackingCode,
		Status:       repository.ShipmentStatusPending,
		Checkpoints:  []repository.ShipmentCheckpoint{initialCheckpoint},
	}

	created, err := s.shipmentRepo.CreateShipment(ctx, shipment)
	if err != nil {
		return repository.Shipment{}, fmt.Errorf("create shipment: %w", err)
	}

	// Update order status to SHIPPED and record tracking number if not yet shipped
	if order.Status != repository.OrderStatusShipped {
		_, _ = s.orderRepo.UpdateOrderStatus(ctx, orderID, repository.OrderStatusShipped, trackingCode)
	}

	s.logger.InfoContext(ctx, "created shipment tracking",
		slog.String("shipment_id", created.ID),
		slog.String("order_id", orderID),
		slog.String("carrier", carrier),
		slog.String("tracking_code", trackingCode),
	)

	return created, nil
}

func (s *OrderService) GetShipmentTracking(ctx context.Context, trackingCode, orderID, shipmentID string) (repository.Shipment, error) {
	if trackingCode != "" {
		return s.shipmentRepo.GetShipmentByTrackingCode(ctx, trackingCode)
	}
	if shipmentID != "" {
		return s.shipmentRepo.GetShipment(ctx, shipmentID)
	}
	if orderID != "" {
		return s.shipmentRepo.GetShipmentByOrderID(ctx, orderID)
	}
	return repository.Shipment{}, errors.New("tracking_code, order_id, or shipment_id must be provided")
}

func (s *OrderService) AddShipmentCheckpoint(ctx context.Context, shipmentID, location, description string) (repository.ShipmentCheckpoint, error) {
	cp := repository.ShipmentCheckpoint{
		ShipmentID:  shipmentID,
		Timestamp:   time.Now(),
		Location:    location,
		Description: description,
		CreatedAt:   time.Now(),
	}
	return s.shipmentRepo.AddShipmentCheckpoint(ctx, cp)
}
