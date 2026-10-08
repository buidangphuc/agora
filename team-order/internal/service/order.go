package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/upstream"
)

var (
	ErrEmptyCart             = errors.New("cart is empty")
	ErrInsufficientStock     = errors.New("insufficient stock for item")
	ErrSelfPurchase          = errors.New("cannot buy your own listing")
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

	// placer places a checkout's orders and binds their reservations atomically
	// (design D6). Resolved in NewOrderService.
	placer repository.OrderPlacer
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
	if s.placer == nil {
		s.placer = defaultPlacer(orderRepo, s.sagaRepo)
	}
	// The in-memory saga store answers "reservations of Cancelled orders" through
	// the order store (Postgres joins); bind it for local/test wiring.
	if b, ok := s.sagaRepo.(interface{ BindOrders(repository.OrderReader) }); ok && orderRepo != nil {
		b.BindOrders(orderRepo)
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
	checkoutOpts ...CheckoutOption,
) ([]repository.Order, error) {
	var cfg checkoutConfig
	for _, opt := range checkoutOpts {
		opt(&cfg)
	}

	// With a client key the saga header is the dedupe record (design D8), taken
	// BEFORE the cart is read: a replay must work after the first attempt emptied
	// the cart, and concurrent same-key requests collapse here, before reserving.
	var saga repository.Saga
	keyed := cfg.idempotencyKey != ""
	if keyed {
		sg, created, err := s.sagaRepo.CreateSaga(ctx, repository.Saga{BuyerID: buyerID, IdempotencyKey: cfg.idempotencyKey})
		if err != nil {
			return nil, fmt.Errorf("create saga: %w", err)
		}
		if !created {
			return s.replayCheckout(ctx, buyerID, sg)
		}
		saga = sg
	}
	// abandon frees the key of a keyed saga whose checkout never reserved anything.
	abandon := func(cause error) ([]repository.Order, error) {
		if keyed {
			bg, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
			defer cancel()
			if err := s.sagaRepo.UpdateSagaStatus(bg, saga.ID, repository.SagaStatusFailed); err != nil {
				s.logger.ErrorContext(ctx, "failed to free the idempotency key of an abandoned checkout",
					slog.String("saga_id", saga.ID), slog.Any("err", err))
			}
		}
		return nil, cause
	}

	cartItems, err := s.cartRepo.GetCart(ctx, buyerID)
	if err != nil {
		return abandon(fmt.Errorf("get cart: %w", err))
	}
	itemsToCheckout := selectItems(cartItems, targetItemIDs)
	if len(itemsToCheckout) == 0 {
		return abandon(ErrEmptyCart)
	}

	// A buyer may not buy their own listing: reject the whole checkout before any
	// stock or voucher is reserved.
	for _, it := range itemsToCheckout {
		if it.SellerID != "" && it.SellerID == buyerID {
			return abandon(ErrSelfPurchase)
		}
	}

	if !keyed {
		// One durable saga header per checkout attempt (AD3).
		sg, _, err := s.sagaRepo.CreateSaga(ctx, repository.Saga{BuyerID: buyerID})
		if err != nil {
			return nil, fmt.Errorf("create saga: %w", err)
		}
		saga = sg
	}
	return s.runCheckout(ctx, saga, buyerID, shippingAddr, itemsToCheckout, paymentMethod, voucherCode)
}

// selectItems returns the cart items to check out: all of them, or only those
// whose id is in targetItemIDs.
func selectItems(cartItems []repository.CartItem, targetItemIDs []string) []repository.CartItem {
	if len(targetItemIDs) == 0 {
		return cartItems
	}
	idMap := make(map[string]struct{}, len(targetItemIDs))
	for _, id := range targetItemIDs {
		idMap[id] = struct{}{}
	}
	var out []repository.CartItem
	for _, it := range cartItems {
		if _, ok := idMap[it.ID]; ok {
			out = append(out, it)
		}
	}
	return out
}

// sellerGroup is one seller's share of a checkout: its cart items, the order id
// they are placed under (pre-generated) and what reserving them produced.
type sellerGroup struct {
	sellerID string
	orderID  string
	items    []repository.CartItem

	reservations   []repository.Reservation
	orderItems     []repository.OrderItem
	itemsSubtotal  int64
	discountAmount int64
	voucherCode    string
}

// groupBySeller splits items by seller (multi-vendor), sorted by seller id so a
// checkout is deterministic, with one pre-generated order id per group.
func groupBySeller(items []repository.CartItem) []*sellerGroup {
	bySeller := map[string]*sellerGroup{}
	var ids []string
	for _, it := range items {
		sellerID := it.SellerID
		if sellerID == "" {
			sellerID = "unknown_seller"
		}
		g, ok := bySeller[sellerID]
		if !ok {
			g = &sellerGroup{sellerID: sellerID, orderID: uuid.NewString()}
			bySeller[sellerID] = g
			ids = append(ids, sellerID)
		}
		g.items = append(g.items, it)
	}
	sort.Strings(ids)
	out := make([]*sellerGroup, 0, len(ids))
	for _, id := range ids {
		out = append(out, bySeller[id])
	}
	return out
}

// runCheckout turns the selected cart items into orders, all of them or none
// (design D6):
//
//	A. reserve every item of every seller group (sorted), plus the voucher hold on
//	   the first group;
//	B. commit every reservation in team-domain, so its TTL sweep can no longer
//	   restore the stock of an order about to exist;
//	C. place every order and bind its reservations in one order_db transaction
//	   (OrderPlacer), which also completes the saga.
//
// Any failure in A or B, or a definite failure in C, compensates every hold of the
// attempt. An ambiguous C error is reconciled by looking the pre-generated order
// ids up before anything is released.
func (s *OrderService) runCheckout(
	ctx context.Context,
	saga repository.Saga,
	buyerID string,
	shippingAddr repository.Address,
	items []repository.CartItem,
	paymentMethod int32,
	voucherCode string,
) ([]repository.Order, error) {
	groups := groupBySeller(items)
	expiresAt := time.Now().Add(s.reservationTTL)
	if paymentMethod <= 0 {
		paymentMethod = 1 // default COD
	}

	// voucherReservationID is the voucher hold placed for this checkout (empty until
	// one is placed). It equals the order id it discounts, so compensation, cancel
	// and the settle-time commit address the same hold with only the order id.
	var voucherReservationID string
	fail := func(cause error) ([]repository.Order, error) {
		s.failAndCompensate(saga.ID, voucherReservationID)
		return nil, cause
	}

	// Phase A — reserve every group.
	for idx, g := range groups {
		if err := s.reserveGroup(ctx, g, buyerID, saga.ID, expiresAt); err != nil {
			return fail(err)
		}
		// Voucher redemption (W1-T2): once per checkout, on the first seller group.
		// A declined voucher aborts the checkout (ErrVoucherRejected).
		if idx == 0 && voucherCode != "" && s.promo != nil {
			discount, verr := s.reserveVoucher(ctx, g.orderID, voucherCode, buyerID, g.itemsSubtotal, g.sellerID)
			if verr != nil {
				return fail(verr)
			}
			g.discountAmount = discount
			g.voucherCode = voucherCode
			voucherReservationID = g.orderID
		}
	}

	// Phase B — make every reservation permanent in team-domain before any order
	// row exists.
	var all []repository.Reservation
	for _, g := range groups {
		all = append(all, g.reservations...)
	}
	if err := s.commitDomainReservations(ctx, all); err != nil {
		return fail(err)
	}

	// Phase C — place every order and bind its reservations in one transaction.
	placed := make([]repository.PlacedOrder, 0, len(groups))
	var checkedOut []string
	for _, g := range groups {
		shippingFee, _, _ := s.CalculateShippingFee(shippingAddr.City, g.itemsSubtotal)
		total := g.itemsSubtotal + shippingFee - g.discountAmount
		if total < 0 {
			total = 0 // a discount never yields a negative total
		}
		resIDs := make([]string, 0, len(g.reservations))
		for _, r := range g.reservations {
			resIDs = append(resIDs, r.ID)
		}
		placed = append(placed, repository.PlacedOrder{
			Order: repository.Order{
				ID:              g.orderID,
				BuyerID:         buyerID,
				SellerID:        g.sellerID,
				Status:          repository.OrderStatusPending,
				TotalAmount:     total,
				ItemsSubtotal:   g.itemsSubtotal,
				ShippingFee:     shippingFee,
				PaymentMethod:   paymentMethod,
				Currency:        "VND",
				ShippingAddress: shippingAddr,
				Items:           g.orderItems,
				VoucherCode:     g.voucherCode,
				DiscountAmount:  g.discountAmount,
			},
			ReservationIDs: resIDs,
		})
		for _, it := range g.items {
			checkedOut = append(checkedOut, it.ID)
		}
	}

	created, perr := s.placer.PlaceOrders(ctx, saga.ID, placed)
	if perr != nil {
		if errors.Is(perr, repository.ErrReservationLost) {
			// Definite: the transaction rolled back because a reservation (or the
			// attempt) is gone. Nothing was placed.
			return fail(fmt.Errorf("place orders: %w", perr))
		}
		// An error is not proof that nothing was written (lost commit ack, deadline):
		// find out before releasing anything.
		s.logger.ErrorContext(ctx, "order placement failed; reconciling before compensating",
			slog.String("saga_id", saga.ID), slog.Any("err", perr))
		existing, outcome := s.reconcilePlacement(placed)
		switch outcome {
		case placementAbsent:
			return fail(fmt.Errorf("place orders: %w", perr))
		case placementComplete:
			s.logger.WarnContext(ctx, "placement reported an error but every order exists; treating the checkout as placed",
				slog.String("saga_id", saga.ID))
			created = existing
		default:
			return nil, fmt.Errorf("%w: %v", ErrPlacementUnknown, perr)
		}
	}

	// The orders exist; removing the checked-out items is best effort.
	s.clearCart(buyerID, checkedOut)
	return created, nil
}

// clearCart removes checked-out items on a fresh context (the orders already
// exist, so a cancelled request must not leave them in the cart).
func (s *OrderService) clearCart(buyerID string, itemIDs []string) {
	if len(itemIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	if err := s.cartRepo.RemoveItems(ctx, buyerID, itemIDs); err != nil {
		s.logger.ErrorContext(ctx, "failed to remove checked-out items from cart",
			slog.String("buyer_id", buyerID),
			slog.Int("item_count", len(itemIDs)),
			slog.Any("err", err),
		)
	}
}

// reserveGroup persists a reservation for every item of the group BEFORE asking
// team-domain to hold its stock (AD3), recording the order items and subtotal.
func (s *OrderService) reserveGroup(ctx context.Context, g *sellerGroup, buyerID, sagaID string, expiresAt time.Time) error {
	for _, it := range g.items {
		res := repository.Reservation{
			ID:        ReservationID(sagaID, it), // attempt-scoped (D7)
			SagaID:    sagaID,
			SellerID:  g.sellerID,
			BuyerID:   buyerID,
			ListingID: it.ListingID,
			VariantID: it.VariantID,
			Quantity:  it.Quantity,
			Status:    repository.ReservationStatusPending,
			ExpiresAt: expiresAt,
		}
		if _, err := s.sagaRepo.CreateReservation(ctx, res); err != nil {
			return fmt.Errorf("persist reservation: %w", err)
		}
		if err := s.reserveStock(ctx, res, it.Title); err != nil {
			return err
		}
		res.Status = repository.ReservationStatusReserved
		g.reservations = append(g.reservations, res)
		g.orderItems = append(g.orderItems, repository.OrderItem{
			ListingID:   it.ListingID,
			VariantID:   it.VariantID,
			Title:       it.Title,
			VariantName: it.VariantName,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			ImageURL:    it.ImageURL,
		})
		g.itemsSubtotal += it.UnitPrice * int64(it.Quantity)
	}
	return nil
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

// UpdateOrderStatus applies a status change requested through the
// UpdateOrderStatus RPC by an actor class (the handler resolves it: the order's
// seller, or an admin acting as seller). Cancelling is never possible here (it
// goes through CancelOrder so stock is released). A target the class may never
// request is ErrActorForbidden; a permitted target from the wrong status is
// ErrInvalidStatus, decided by the compare-and-set write itself.
func (s *OrderService) UpdateOrderStatus(ctx context.Context, id string, actor Actor, to repository.OrderStatus, trackingNumber string) (repository.Order, error) {
	if to == repository.OrderStatusCancelled {
		return repository.Order{}, ErrActorForbidden
	}
	from := AllowedFrom(to, actor)
	if len(from) == 0 {
		return repository.Order{}, ErrActorForbidden
	}
	updated, err := s.orderRepo.UpdateOrderStatusFrom(ctx, id, to, from, trackingNumber)
	if errors.Is(err, repository.ErrStatusConflict) {
		return repository.Order{}, fmt.Errorf("%w: order cannot move to %v from its current status", ErrInvalidStatus, to)
	}
	return updated, err
}

// CancelResult is a won cancel: the cancelled order, and whether a stock release
// is still outstanding (parked RELEASE_FAILED for the sweep). The order IS
// Cancelled either way; ReleasePending only says the stock is not back yet.
type CancelResult struct {
	repository.Order
	ReleasePending bool
}

// CancelOrder claims Cancelled FIRST with the compare-and-set write (from Pending
// or Paid, design D10). Only the caller that wins the claim then releases the
// order's own reservations by their original ids and the voucher hold placed for
// it, so concurrent cancels restore stock once and a Shipped/Completed/Cancelled
// order releases nothing (ErrInvalidStatus). A failing release is parked
// RELEASE_FAILED and retried by the sweep, which also releases reservations still
// held by a Cancelled order (a crash between claim and release). A voucher release
// failure is logged and never fails the cancel.
func (s *OrderService) CancelOrder(ctx context.Context, id string) (CancelResult, error) {
	claimed, err := s.orderRepo.UpdateOrderStatusFrom(ctx, id, repository.OrderStatusCancelled, cancelFrom, "")
	if err != nil {
		if errors.Is(err, repository.ErrStatusConflict) {
			msg := "order is not in a cancellable status"
			if cur, gerr := s.orderRepo.GetOrder(ctx, id); gerr == nil {
				msg = fmt.Sprintf("cannot cancel order in status %v", cur.Status)
			}
			return CancelResult{}, fmt.Errorf("%w: %s", ErrInvalidStatus, msg)
		}
		return CancelResult{}, err
	}

	// Everything after the claim runs on a fresh context (AD3): the request may be
	// gone, and neither release may be abandoned because of it. The budget is the
	// short inline one: a release that cannot finish in time is parked and the
	// sweep retries it, so the buyer's cancel still succeeds.
	bg, cancel := context.WithTimeout(context.Background(), s.releaseCfg.inlineTimeout)
	defer cancel()

	pending, rerr := s.releaseOrderReservations(bg, claimed)
	if rerr != nil {
		// The order is Cancelled; its reservations still hold stock and the sweep
		// releases them. Never fail a cancel the buyer has been granted.
		s.logger.ErrorContext(ctx, "order cancelled but its release could not be recorded; the sweep will release it",
			slog.String("order_id", claimed.ID), slog.Any("err", rerr))
		pending = true
	}
	if claimed.VoucherCode != "" {
		s.releaseVoucher(bg, claimed.ID) // the hold id is the order id
	}
	return CancelResult{Order: claimed, ReleasePending: pending}, nil
}

// releaseOrderReservations releases every reservation of the order that still
// holds stock (COMMITTED, or parked RELEASE_FAILED) by its original id. pending is
// true when any release was parked for the sweep.
func (s *OrderService) releaseOrderReservations(ctx context.Context, order repository.Order) (pending bool, err error) {
	reservations, err := s.sagaRepo.ListReservationsByOrder(ctx, order.ID)
	if err != nil {
		return true, fmt.Errorf("load reservations of order %s: %w", order.ID, err)
	}
	if len(reservations) == 0 && len(order.Items) > 0 {
		s.logger.WarnContext(ctx, "cancelled order has no reservations to release (placed before reservation tracking?)",
			slog.String("order_id", order.ID))
	}
	for _, res := range reservations {
		if res.Status != repository.ReservationStatusCommitted && res.Status != repository.ReservationStatusReleaseFailed {
			continue
		}
		released, perr := s.releaseOrPark(ctx, res)
		if perr != nil {
			return true, perr
		}
		if !released {
			pending = true
		}
	}
	return pending, nil
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

	// A compare-and-set from the status read above: a concurrent transition that
	// got there first makes this one lose (ErrInvalidReturnStatus).
	updated, err := s.returnRepo.TransitionReturn(ctx, id, existing.Status, newStatus)
	if err != nil {
		if errors.Is(err, repository.ErrReturnStatusConflict) {
			return repository.OrderReturn{}, fmt.Errorf("%w: return status changed concurrently", ErrInvalidReturnStatus)
		}
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

// CreateShipment claims Shipped FIRST with the compare-and-set write (from Pending
// for a cash-on-delivery hand-over, or Paid) and creates the shipment, with its
// OrderShipped outbox row, only if that claim won (design D12). An order in any
// other status is ErrInvalidStatus and gets no shipment. A shipment insert that
// fails after a won claim is returned (INTERNAL) and logged for a manual fix; a
// retry then conflicts because the order is already Shipped.
func (s *OrderService) CreateShipment(ctx context.Context, orderID, carrier, trackingCode string) (repository.Shipment, error) {
	if orderID == "" {
		return repository.Shipment{}, repository.ErrOrderNotFound
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

	order, err := s.orderRepo.UpdateOrderStatusFrom(ctx, orderID, repository.OrderStatusShipped,
		AllowedFrom(repository.OrderStatusShipped, ActorSeller), trackingCode)
	if err != nil {
		if errors.Is(err, repository.ErrStatusConflict) {
			msg := "order cannot be shipped in its current status"
			if cur, gerr := s.orderRepo.GetOrder(ctx, orderID); gerr == nil {
				msg = fmt.Sprintf("order in status %v cannot be shipped", cur.Status)
			}
			return repository.Shipment{}, fmt.Errorf("%w: %s", ErrInvalidStatus, msg)
		}
		return repository.Shipment{}, err
	}

	now := time.Now()
	shipment := repository.Shipment{
		OrderID:      orderID,
		Carrier:      carrier,
		TrackingCode: trackingCode,
		Status:       repository.ShipmentStatusPending,
		Checkpoints: []repository.ShipmentCheckpoint{{
			Timestamp:   now,
			Location:    "Trung tâm phân loại & Bưu cục tiếp nhận",
			Description: fmt.Sprintf("Người bán đã bàn giao kiện hàng cho đơn vị vận chuyển %s", carrier),
			CreatedAt:   now,
		}},
		BuyerID:  order.BuyerID,
		SellerID: order.SellerID,
	}
	created, err := s.shipmentRepo.CreateShipment(ctx, shipment)
	if err != nil {
		s.logger.ErrorContext(ctx, "order marked Shipped but its shipment could not be created; fix by hand",
			slog.String("order_id", orderID), slog.String("tracking_code", trackingCode), slog.Any("err", err))
		return repository.Shipment{}, fmt.Errorf("create shipment: %w", err)
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
