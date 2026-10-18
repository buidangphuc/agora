package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
)

var ErrOrderNotFound = errors.New("order not found")

// ErrStatusConflict is returned by UpdateOrderStatusFrom when the order exists but
// is no longer in one of the allowed source statuses at the moment of the write.
var ErrStatusConflict = errors.New("order status changed concurrently")

type OrderStatus int32

const (
	OrderStatusUnspecified OrderStatus = 0
	OrderStatusPending     OrderStatus = 1
	OrderStatusPaid        OrderStatus = 2
	OrderStatusShipped     OrderStatus = 3
	OrderStatusCompleted   OrderStatus = 4
	OrderStatusCancelled   OrderStatus = 5
)

type Address struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	Street        string `json:"street"`
	Ward          string `json:"ward"`
	District      string `json:"district"`
	City          string `json:"city"`
	IsDefault     bool   `json:"is_default"`
}

type OrderItem struct {
	ID          string
	OrderID     string
	ListingID   string
	VariantID   string
	Title       string
	VariantName string
	Quantity    int32
	UnitPrice   int64
	ImageURL    string
}

type Order struct {
	ID              string
	BuyerID         string
	SellerID        string
	Status          OrderStatus
	TotalAmount     int64
	Currency        string
	ShippingAddress Address
	Items           []OrderItem
	TrackingNumber  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ShippingFee     int64
	PaymentMethod   int32
	ItemsSubtotal   int64
	VoucherCode     string
	DiscountAmount  int64
	// PaidAt is when the Pending -> Paid compare-and-set succeeded; nil when the
	// order was never paid or was paid before migration 0007.
	PaidAt *time.Time
	// PaymentDueAt is the deadline of an unpaid online-pay order, snapshotted at
	// placement; nil for COD and for orders placed before migration 0008.
	PaymentDueAt *time.Time
	// CancelReason is the closed orderv1.CancelReason code stored by the claim that
	// won the move to Cancelled; 0 = unspecified.
	CancelReason int32
	// PaymentReminderSentAt is stamped once by MarkReminderSent when the payment
	// reminder is emitted; nil until then.
	PaymentReminderSentAt *time.Time
	// ShippedAt is stamped by the move to Shipped; AutoCompleteAt is ShippedAt plus
	// the auto-complete duration and is cleared by the move to Completed.
	ShippedAt      *time.Time
	AutoCompleteAt *time.Time
	// CompletedAt and ReturnWindowEndsAt are stamped by the move to Completed.
	CompletedAt        *time.Time
	ReturnWindowEndsAt *time.Time
	// ShipBy is the seller's ship-by deadline: stamped at payment (online) or at
	// placement (COD) and never moved afterwards.
	ShipBy *time.Time
	// CompletedBy is the orderv1.CompletedBy of the write that won the move to
	// Completed; it is set on the row that write returns and is not persisted.
	CompletedBy int32
}

type OrderRepository interface {
	CreateOrder(ctx context.Context, order Order) (Order, error)
	GetOrder(ctx context.Context, id string) (Order, error)
	ListBuyerOrders(ctx context.Context, buyerID string, statusFilter int32) ([]Order, error)
	ListSellerOrders(ctx context.Context, sellerID string, statusFilter int32) ([]Order, error)
	// UpdateOrderStatusFrom is the only status write: a compare-and-set that moves
	// the order to `to` only if its status at the moment of the write is one of
	// allowedFrom. It returns ErrOrderNotFound or ErrStatusConflict otherwise. A move
	// to Paid records paid_at and writes the OrderPaid outbox row in the same
	// transaction; a won move to Cancelled writes the OrderCancelled outbox row in
	// the same transaction. trackingNumber, when non-empty, is stored with the change.
	UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string, opts ...StatusOption) (Order, error)
	// FindReminderDue returns up to limit Pending online-pay orders with a deadline,
	// not yet reminded, whose reminder window is open: deadline-before <= now <
	// deadline (inclusive start, exclusive deadline), earliest deadline first.
	FindReminderDue(ctx context.Context, now time.Time, before time.Duration, limit int) ([]Order, error)
	// MarkReminderSent is the once-only compare-and-set: it stamps
	// payment_reminder_sent_at = now and writes the OrderPaymentReminder outbox row
	// in the same transaction, only if the order is still Pending and unstamped.
	// Otherwise it returns ErrOrderNotFound or ErrStatusConflict and writes nothing.
	MarkReminderSent(ctx context.Context, id string, now time.Time) (Order, error)
}

// ReminderOutboxBuilder turns an order that has just been stamped as reminded
// into the OrderPaymentReminder outbox row written in the same transaction.
type ReminderOutboxBuilder func(order Order) (OutboxRow, error)

// WithReminderOutbox makes every won MarkReminderSent write the reminder outbox
// row built by b in the SAME transaction as the stamp.
func WithReminderOutbox(b ReminderOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.reminderOutbox = b }
}

// StatusOption tunes one UpdateOrderStatusFrom write.
type StatusOption func(*statusConfig)

type statusConfig struct{ cancelReason, completedBy int32 }

// WithCompletedBy records who completed the order (an orderv1.CompletedBy) when the
// write wins a move to Completed; it is ignored for every other move.
func WithCompletedBy(by int32) StatusOption {
	return func(c *statusConfig) { c.completedBy = by }
}

// WithCancelReason stores reason on the order when the write wins a move to
// Cancelled; it is ignored for every other move.
func WithCancelReason(reason int32) StatusOption {
	return func(c *statusConfig) { c.cancelReason = reason }
}

func newStatusConfig(opts []StatusOption) statusConfig {
	var c statusConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// PaidOutboxBuilder turns an order that has just transitioned to PAID into the
// outbox row to persist alongside that transition (ADR-0013). order.UpdatedAt is
// the transition timestamp. The builder is injected (internal/events owns the
// envelope/contract) so the repository stays free of an events dependency.
type PaidOutboxBuilder func(order Order) (OutboxRow, error)

// CancelledOutboxBuilder turns an order that has just won the claim to CANCELLED
// into the outbox row to persist alongside that claim. order is the row the
// compare-and-set returned: PaidAt is set iff the order was cancelled from Paid
// (paid_at is written only by the move to Paid, and only Pending/Paid cancel).
type CancelledOutboxBuilder func(order Order) (OutboxRow, error)

// OrderRepoOption customizes an order repository.
// PlacedOutboxBuilder turns an order that has just been placed into the OrderPlaced
// outbox row written in the placement transaction.
type PlacedOutboxBuilder func(order Order) (OutboxRow, error)

// WithPlacedOutbox makes PlaceOrders write one OrderPlaced outbox row per order in
// the same transaction as the placement.
func WithPlacedOutbox(b PlacedOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.placedOutbox = b }
}

type OrderRepoOption func(*orderRepoConfig)

// CompletedOutboxBuilder turns an order that has just won the move to Completed
// into the OrderCompleted outbox row written in the same transaction.
type CompletedOutboxBuilder func(order Order) (OutboxRow, error)

// WithCompletedOutbox makes every won move to Completed write the row built by b in
// the SAME transaction as the status change.
func WithCompletedOutbox(b CompletedOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.completedOutbox = b }
}

// WithAutoCompleteAfter sets how long after shipping auto_complete_at is stamped.
func WithAutoCompleteAfter(d time.Duration) OrderRepoOption {
	return func(c *orderRepoConfig) { c.autoCompleteAfter = d }
}

// WithReturnWindow sets how long after completion the return window stays open.
func WithReturnWindow(d time.Duration) OrderRepoOption {
	return func(c *orderRepoConfig) { c.returnWindow = d }
}

// WithShipByAfter sets how long after payment (or COD placement) ship_by is stamped.
func WithShipByAfter(d time.Duration) OrderRepoOption {
	return func(c *orderRepoConfig) { c.shipByAfter = d }
}

// durSecs returns d in seconds, or nil (SQL NULL) when d is not configured.
func durSecs(d time.Duration) *float64 {
	if d <= 0 {
		return nil
	}
	s := d.Seconds()
	return &s
}

func stampAfter(t time.Time, d time.Duration) *time.Time {
	if d <= 0 {
		return nil
	}
	v := t.Add(d)
	return &v
}

type orderRepoConfig struct {
	paidOutbox      PaidOutboxBuilder
	cancelledOutbox CancelledOutboxBuilder
	placedOutbox    PlacedOutboxBuilder
	reminderOutbox  ReminderOutboxBuilder
	completedOutbox CompletedOutboxBuilder
	outbox          OutboxRepository // in-memory repo only

	autoCompleteAfter, returnWindow, shipByAfter time.Duration
}

// WithPaidOutbox makes every first transition to PAID write an outbox row built
// by b in the SAME transaction as the status update. If building or enqueueing
// the row fails, the status change is rolled back too.
func WithPaidOutbox(b PaidOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.paidOutbox = b }
}

// WithCancelledOutbox makes every won claim to CANCELLED write an outbox row
// built by b in the SAME transaction as the status update. A lost claim writes
// nothing; if building or enqueueing the row fails, the cancel is rolled back too.
func WithCancelledOutbox(b CancelledOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.cancelledOutbox = b }
}

// WithInMemoryOutbox sets the outbox store the in-memory repository writes to,
// atomically with the status change. Ignored by the Postgres repository, which
// always writes to order_outbox_events in its own transaction.
func WithInMemoryOutbox(o OutboxRepository) OrderRepoOption {
	return func(c *orderRepoConfig) { c.outbox = o }
}

func newOrderRepoConfig(opts []OrderRepoOption) orderRepoConfig {
	var c orderRepoConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

type PostgresOrderRepository struct {
	pool            *pgxpool.Pool
	paidOutbox      PaidOutboxBuilder
	cancelledOutbox CancelledOutboxBuilder
	placedOutbox    PlacedOutboxBuilder
	reminderOutbox  ReminderOutboxBuilder
	completedOutbox CompletedOutboxBuilder

	autoCompleteAfter, returnWindow, shipByAfter time.Duration
}

func NewPostgresOrderRepository(pool *pgxpool.Pool, opts ...OrderRepoOption) *PostgresOrderRepository {
	cfg := newOrderRepoConfig(opts)
	return &PostgresOrderRepository{pool: pool, paidOutbox: cfg.paidOutbox, cancelledOutbox: cfg.cancelledOutbox, placedOutbox: cfg.placedOutbox, reminderOutbox: cfg.reminderOutbox,
		completedOutbox: cfg.completedOutbox, autoCompleteAfter: cfg.autoCompleteAfter, returnWindow: cfg.returnWindow, shipByAfter: cfg.shipByAfter}
}

const orderColumns = `id, buyer_id, seller_id, status, total_amount, currency, shipping_address, tracking_number, created_at, updated_at, shipping_fee, items_subtotal, payment_method, voucher_code, discount_amount, paid_at, payment_due_at, cancel_reason, payment_reminder_sent_at, shipped_at, auto_complete_at, completed_at, return_window_ends_at, ship_by`
const orderItemColumns = `id, order_id, listing_id, variant_id, title, variant_name, quantity, unit_price, image_url`

func scanOrder(row pgx.Row, o *Order) error {
	var addrRaw []byte
	var statusInt int32
	var paidAt, dueAt, remindedAt *time.Time
	if err := row.Scan(&o.ID, &o.BuyerID, &o.SellerID, &statusInt, &o.TotalAmount, &o.Currency, &addrRaw, &o.TrackingNumber, &o.CreatedAt, &o.UpdatedAt, &o.ShippingFee, &o.ItemsSubtotal, &o.PaymentMethod, &o.VoucherCode, &o.DiscountAmount, &paidAt, &dueAt, &o.CancelReason, &remindedAt, &o.ShippedAt, &o.AutoCompleteAt, &o.CompletedAt, &o.ReturnWindowEndsAt, &o.ShipBy); err != nil {
		return err
	}
	o.PaidAt = paidAt
	o.PaymentDueAt = dueAt
	o.PaymentReminderSentAt = remindedAt
	o.Status = OrderStatus(statusInt)
	if len(addrRaw) > 0 {
		_ = json.Unmarshal(addrRaw, &o.ShippingAddress)
	}
	return nil
}

func (r *PostgresOrderRepository) CreateOrder(ctx context.Context, order Order) (Order, error) {
	if order.ID == "" {
		order.ID = uuid.NewString()
	}
	if order.Currency == "" {
		order.Currency = "VND"
	}
	if order.Status == 0 {
		order.Status = OrderStatusPending
	}
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := insertOrderTx(ctx, tx, &order); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return order, nil
}

// insertOrderTx inserts an order (ID, Currency, Status and timestamps already set)
// and its items inside tx, filling the item ids.
func insertOrderTx(ctx context.Context, tx pgx.Tx, order *Order) error {
	addrBytes, err := json.Marshal(order.ShippingAddress)
	if err != nil {
		return fmt.Errorf("marshal address: %w", err)
	}
	const qOrder = `INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, currency, shipping_address, tracking_number, created_at, updated_at, shipping_fee, items_subtotal, payment_method, voucher_code, discount_amount, payment_due_at, ship_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`
	if _, err := tx.Exec(ctx, qOrder, order.ID, order.BuyerID, order.SellerID, int32(order.Status), order.TotalAmount, order.Currency, addrBytes, order.TrackingNumber, order.CreatedAt, order.UpdatedAt, order.ShippingFee, order.ItemsSubtotal, order.PaymentMethod, order.VoucherCode, order.DiscountAmount, order.PaymentDueAt, order.ShipBy); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	for i := range order.Items {
		item := &order.Items[i]
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		item.OrderID = order.ID
		const qItem = `INSERT INTO order_items (id, order_id, listing_id, variant_id, title, variant_name, quantity, unit_price, image_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		if _, err := tx.Exec(ctx, qItem, item.ID, item.OrderID, item.ListingID, item.VariantID, item.Title, item.VariantName, item.Quantity, item.UnitPrice, item.ImageURL); err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
	}
	return nil
}

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type rowQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (r *PostgresOrderRepository) loadItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	return loadOrderItems(ctx, r.pool, orderID)
}

func loadOrderItems(ctx context.Context, q rowQuerier, orderID string) ([]OrderItem, error) {
	const sqlQ = `SELECT ` + orderItemColumns + ` FROM order_items WHERE order_id = $1`
	rows, err := q.Query(ctx, sqlQ, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var item OrderItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ListingID, &item.VariantID, &item.Title, &item.VariantName, &item.Quantity, &item.UnitPrice, &item.ImageURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresOrderRepository) GetOrder(ctx context.Context, id string) (Order, error) {
	const q = `SELECT ` + orderColumns + ` FROM orders WHERE id = $1`
	var o Order
	if err := scanOrder(r.pool.QueryRow(ctx, q, id), &o); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("get order %q: %w", id, err)
	}
	items, err := r.loadItems(ctx, o.ID)
	if err != nil {
		return Order{}, fmt.Errorf("load items for order %q: %w", id, err)
	}
	o.Items = items
	return o, nil
}

func (r *PostgresOrderRepository) ListBuyerOrders(ctx context.Context, buyerID string, statusFilter int32) ([]Order, error) {
	var q string
	var args []any
	if statusFilter > 0 {
		q = `SELECT ` + orderColumns + ` FROM orders WHERE buyer_id = $1 AND status = $2 ORDER BY created_at DESC`
		args = []any{buyerID, statusFilter}
	} else {
		q = `SELECT ` + orderColumns + ` FROM orders WHERE buyer_id = $1 ORDER BY created_at DESC`
		args = []any{buyerID}
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list buyer orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range orders {
		items, err := r.loadItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}
	return orders, nil
}

func (r *PostgresOrderRepository) ListSellerOrders(ctx context.Context, sellerID string, statusFilter int32) ([]Order, error) {
	var q string
	var args []any
	if statusFilter > 0 {
		q = `SELECT ` + orderColumns + ` FROM orders WHERE seller_id = $1 AND status = $2 ORDER BY created_at DESC`
		args = []any{sellerID, statusFilter}
	} else {
		q = `SELECT ` + orderColumns + ` FROM orders WHERE seller_id = $1 ORDER BY created_at DESC`
		args = []any{sellerID}
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list seller orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range orders {
		items, err := r.loadItems(ctx, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}
	return orders, nil
}

func statusInts(st []OrderStatus) []int32 {
	out := make([]int32, len(st))
	for i, x := range st {
		out[i] = int32(x)
	}
	return out
}

func (r *PostgresOrderRepository) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string, opts ...StatusOption) (Order, error) {
	cfg := newStatusConfig(opts)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// The status condition is decided by this UPDATE itself (row lock), never by
	// an earlier read. paid_at is recorded by the move to Paid and nothing else.
	const q = `UPDATE orders SET status = $2, updated_at = now(),
			tracking_number = CASE WHEN $4 <> '' THEN $4 ELSE tracking_number END,
			paid_at = CASE WHEN $2 = 2 THEN now() ELSE paid_at END,
			cancel_reason = CASE WHEN $2 = 5 THEN $5::int ELSE cancel_reason END,
			ship_by = CASE WHEN $2 = 2 THEN now() + make_interval(secs => $6::float8) ELSE ship_by END,
			shipped_at = CASE WHEN $2 = 3 THEN now() ELSE shipped_at END,
			auto_complete_at = CASE WHEN $2 = 3 THEN now() + make_interval(secs => $7::float8)
			                        WHEN $2 = 4 THEN NULL ELSE auto_complete_at END,
			completed_at = CASE WHEN $2 = 4 THEN now() ELSE completed_at END,
			return_window_ends_at = CASE WHEN $2 = 4 THEN now() + make_interval(secs => $8::float8) ELSE return_window_ends_at END
		WHERE id = $1 AND status = ANY($3)
		RETURNING ` + orderColumns
	var o Order
	if err := scanOrder(tx.QueryRow(ctx, q, id, int32(to), statusInts(allowedFrom), trackingNumber, cfg.cancelReason, durSecs(r.shipByAfter), durSecs(r.autoCompleteAfter), durSecs(r.returnWindow)), &o); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if qerr := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1)`, id).Scan(&exists); qerr != nil {
				return Order{}, fmt.Errorf("check order %q: %w", id, qerr)
			}
			if !exists {
				return Order{}, ErrOrderNotFound
			}
			return Order{}, ErrStatusConflict
		}
		return Order{}, fmt.Errorf("update order status: %w", err)
	}
	items, err := loadOrderItems(ctx, tx, o.ID)
	if err != nil {
		return Order{}, err
	}
	o.Items = items

	// Domain-fact event (ADR-0013): the order.events row commits or rolls back with
	// the status change. Only Pending -> Paid reaches here with to == Paid, so it is
	// written once per order.
	if to == OrderStatusPaid && r.paidOutbox != nil {
		row, err := r.paidOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build order paid outbox row: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, row); err != nil {
			return Order{}, err
		}
	}
	// Only the caller that won the claim reaches here, so an order is cancelled
	// (and its OrderCancelled fact written) at most once.
	if to == OrderStatusCancelled && r.cancelledOutbox != nil {
		row, err := r.cancelledOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build order cancelled outbox row: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, row); err != nil {
			return Order{}, err
		}
	}
	if to == OrderStatusCompleted {
		o.CompletedBy = cfg.completedBy
		if r.completedOutbox != nil {
			row, err := r.completedOutbox(o)
			if err != nil {
				return Order{}, fmt.Errorf("build order completed outbox row: %w", err)
			}
			if err := enqueueOutboxTx(ctx, tx, row); err != nil {
				return Order{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return o, nil
}

// InMemoryOrderRepository for unit tests
type InMemoryOrderRepository struct {
	mu              sync.RWMutex
	orders          map[string]Order
	paidOutbox      PaidOutboxBuilder
	cancelledOutbox CancelledOutboxBuilder
	placedOutbox    PlacedOutboxBuilder
	reminderOutbox  ReminderOutboxBuilder
	completedOutbox CompletedOutboxBuilder
	outbox          OutboxRepository
	returnSource    ReturnRepository

	autoCompleteAfter, returnWindow, shipByAfter time.Duration
}

func NewInMemoryOrderRepository(opts ...OrderRepoOption) *InMemoryOrderRepository {
	cfg := newOrderRepoConfig(opts)
	return &InMemoryOrderRepository{orders: make(map[string]Order), paidOutbox: cfg.paidOutbox,
		cancelledOutbox: cfg.cancelledOutbox, placedOutbox: cfg.placedOutbox, reminderOutbox: cfg.reminderOutbox, outbox: cfg.outbox,
		completedOutbox: cfg.completedOutbox, autoCompleteAfter: cfg.autoCompleteAfter, returnWindow: cfg.returnWindow, shipByAfter: cfg.shipByAfter}
}

func (r *InMemoryOrderRepository) CreateOrder(_ context.Context, order Order) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if order.ID == "" {
		order.ID = uuid.NewString()
	}
	if order.Currency == "" {
		order.Currency = "VND"
	}
	if order.Status == 0 {
		order.Status = OrderStatusPending
	}
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()
	for i := range order.Items {
		if order.Items[i].ID == "" {
			order.Items[i].ID = uuid.NewString()
		}
		order.Items[i].OrderID = order.ID
	}
	r.orders[order.ID] = order
	return order, nil
}

func (r *InMemoryOrderRepository) GetOrder(_ context.Context, id string) (Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	return o, nil
}

func (r *InMemoryOrderRepository) ListBuyerOrders(_ context.Context, buyerID string, statusFilter int32) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []Order
	for _, o := range r.orders {
		if o.BuyerID == buyerID {
			if statusFilter == 0 || int32(o.Status) == statusFilter {
				res = append(res, o)
			}
		}
	}
	return res, nil
}

func (r *InMemoryOrderRepository) ListSellerOrders(_ context.Context, sellerID string, statusFilter int32) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []Order
	for _, o := range r.orders {
		if o.SellerID == sellerID {
			if statusFilter == 0 || int32(o.Status) == statusFilter {
				res = append(res, o)
			}
		}
	}
	return res, nil
}

func (r *InMemoryOrderRepository) UpdateOrderStatusFrom(ctx context.Context, id string, to OrderStatus, allowedFrom []OrderStatus, trackingNumber string, opts ...StatusOption) (Order, error) {
	cfg := newStatusConfig(opts)
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	allowed := false
	for _, st := range allowedFrom {
		if o.Status == st {
			allowed = true
			break
		}
	}
	if !allowed {
		return Order{}, ErrStatusConflict
	}
	o.Status = to
	if trackingNumber != "" {
		o.TrackingNumber = trackingNumber
	}
	o.UpdatedAt = time.Now()
	if to == OrderStatusCancelled {
		o.CancelReason = cfg.cancelReason
	}
	if to == OrderStatusPaid {
		paidAt := o.UpdatedAt
		o.PaidAt = &paidAt
		o.ShipBy = stampAfter(paidAt, r.shipByAfter)
	}
	if to == OrderStatusShipped {
		shippedAt := o.UpdatedAt
		o.ShippedAt = &shippedAt
		o.AutoCompleteAt = stampAfter(shippedAt, r.autoCompleteAfter)
	}
	if to == OrderStatusCompleted {
		completedAt := o.UpdatedAt
		o.CompletedAt = &completedAt
		o.AutoCompleteAt = nil
		o.ReturnWindowEndsAt = stampAfter(completedAt, r.returnWindow)
		o.CompletedBy = cfg.completedBy
	}
	// Mirror the Postgres repo: the outbox row and the status change are one unit;
	// the order is stored only after the row is enqueued.
	if to == OrderStatusPaid && r.paidOutbox != nil && r.outbox != nil {
		row, err := r.paidOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build order paid outbox row: %w", err)
		}
		if err := r.outbox.Enqueue(ctx, row); err != nil {
			return Order{}, err
		}
	}
	if to == OrderStatusCancelled && r.cancelledOutbox != nil && r.outbox != nil {
		row, err := r.cancelledOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build order cancelled outbox row: %w", err)
		}
		if err := r.outbox.Enqueue(ctx, row); err != nil {
			return Order{}, err
		}
	}
	if to == OrderStatusCompleted && r.completedOutbox != nil && r.outbox != nil {
		row, err := r.completedOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build order completed outbox row: %w", err)
		}
		if err := r.outbox.Enqueue(ctx, row); err != nil {
			return Order{}, err
		}
	}
	r.orders[id] = o
	return o, nil
}

// ExpiryFinder is implemented by order stores that can list unpaid orders past
// their payment deadline. FindExpired returns up to limit Pending online-pay
// orders whose deadline is strictly before now, earliest deadline first.
type ExpiryFinder interface {
	FindExpired(ctx context.Context, now time.Time, limit int) ([]Order, error)
}

// AutoCompleteFinder is implemented by order stores that can list SHIPPED orders
// whose auto_complete_at has passed and that no return blocks (a PENDING return,
// or an APPROVED one on an online-paid order), oldest deadline first.
type AutoCompleteFinder interface {
	ListAutoCompletable(ctx context.Context, now time.Time, limit int) ([]Order, error)
}

func (r *PostgresOrderRepository) ListAutoCompletable(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	const q = `SELECT ` + orderColumns + ` FROM orders o
		WHERE o.status = $1 AND o.auto_complete_at IS NOT NULL AND o.auto_complete_at <= $2
		  AND NOT EXISTS (SELECT 1 FROM order_returns r WHERE r.order_id = o.id
		                  AND (r.status = $3 OR (r.status = $4 AND o.paid_at IS NOT NULL)))
		ORDER BY o.auto_complete_at, o.id LIMIT $5`
	rows, err := r.pool.Query(ctx, q, int32(OrderStatusShipped), now, int32(ReturnStatusPending), int32(ReturnStatusApproved), limit)
	if err != nil {
		return nil, fmt.Errorf("list auto-completable orders: %w", err)
	}
	defer rows.Close()
	var res []Order
	for rows.Next() {
		var o Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, err
		}
		res = append(res, o)
	}
	return res, rows.Err()
}

// SetReturnSource lets the in-memory order store evaluate the blocking-return
// rule of ListAutoCompletable (the Postgres store does it in SQL).
func (r *InMemoryOrderRepository) SetReturnSource(src ReturnRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.returnSource = src
}

// ListAutoCompletable: SHIPPED orders with auto_complete_at <= now and no blocking
// return (PENDING, or APPROVED when the order has paid_at), oldest deadline first.
func (r *InMemoryOrderRepository) ListAutoCompletable(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	r.mu.RLock()
	var due []Order
	for _, o := range r.orders {
		if o.Status == OrderStatusShipped && o.AutoCompleteAt != nil && !o.AutoCompleteAt.After(now) {
			due = append(due, o)
		}
	}
	src := r.returnSource
	r.mu.RUnlock()
	sort.Slice(due, func(i, j int) bool {
		if !due[i].AutoCompleteAt.Equal(*due[j].AutoCompleteAt) {
			return due[i].AutoCompleteAt.Before(*due[j].AutoCompleteAt)
		}
		return due[i].ID < due[j].ID
	})
	var res []Order
	for _, o := range due {
		if limit > 0 && len(res) >= limit {
			break
		}
		if src != nil {
			rets, err := src.ListReturnsByOrder(ctx, o.ID)
			if err != nil {
				return nil, err
			}
			blocked := false
			for _, ret := range rets {
				if ret.Status == ReturnStatusPending || (ret.Status == ReturnStatusApproved && o.PaidAt != nil) {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
		}
		res = append(res, o)
	}
	return res, nil
}

// codPaymentMethod is paymentv1.PaymentMethod_PAYMENT_METHOD_COD; COD orders have
// no payment deadline and are never reminded.
const codPaymentMethod = int32(paymentv1.PaymentMethod_PAYMENT_METHOD_COD)

func (r *PostgresOrderRepository) FindReminderDue(ctx context.Context, now time.Time, before time.Duration, limit int) ([]Order, error) {
	const q = `SELECT ` + orderColumns + ` FROM orders
		WHERE status = $1 AND payment_method <> $2 AND payment_due_at IS NOT NULL
		  AND payment_reminder_sent_at IS NULL
		  AND payment_due_at - make_interval(secs => $3) <= $4 AND payment_due_at > $4
		ORDER BY payment_due_at, id LIMIT $5`
	rows, err := r.pool.Query(ctx, q, int32(OrderStatusPending), codPaymentMethod, before.Seconds(), now, limit)
	if err != nil {
		return nil, fmt.Errorf("find reminder due: %w", err)
	}
	defer rows.Close()
	var res []Order
	for rows.Next() {
		var o Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		res = append(res, o)
	}
	return res, rows.Err()
}

func (r *PostgresOrderRepository) FindExpired(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	const q = `SELECT ` + orderColumns + ` FROM orders
		WHERE status = $1 AND payment_method <> $2 AND payment_due_at IS NOT NULL AND payment_due_at < $3
		ORDER BY payment_due_at, id LIMIT $4`
	rows, err := r.pool.Query(ctx, q, int32(OrderStatusPending), codPaymentMethod, now, limit)
	if err != nil {
		return nil, fmt.Errorf("find expired: %w", err)
	}
	defer rows.Close()
	var res []Order
	for rows.Next() {
		var o Order
		if err := scanOrder(rows, &o); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		res = append(res, o)
	}
	return res, rows.Err()
}

func (r *PostgresOrderRepository) MarkReminderSent(ctx context.Context, id string, now time.Time) (Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	const q = `UPDATE orders SET payment_reminder_sent_at = $2
		WHERE id = $1 AND status = $3 AND payment_reminder_sent_at IS NULL
		RETURNING ` + orderColumns
	var o Order
	if err := scanOrder(tx.QueryRow(ctx, q, id, now, int32(OrderStatusPending)), &o); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if qerr := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1)`, id).Scan(&exists); qerr != nil {
				return Order{}, fmt.Errorf("check order %q: %w", id, qerr)
			}
			if !exists {
				return Order{}, ErrOrderNotFound
			}
			return Order{}, ErrStatusConflict
		}
		return Order{}, fmt.Errorf("mark reminder sent: %w", err)
	}
	items, err := loadOrderItems(ctx, tx, o.ID)
	if err != nil {
		return Order{}, err
	}
	o.Items = items
	if r.reminderOutbox != nil {
		row, err := r.reminderOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build payment reminder outbox row: %w", err)
		}
		if err := enqueueOutboxTx(ctx, tx, row); err != nil {
			return Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return o, nil
}

func (r *InMemoryOrderRepository) FindReminderDue(_ context.Context, now time.Time, before time.Duration, limit int) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []Order
	for _, o := range r.orders {
		if o.Status != OrderStatusPending || o.PaymentMethod == codPaymentMethod || o.PaymentDueAt == nil ||
			o.PaymentReminderSentAt != nil {
			continue
		}
		due := *o.PaymentDueAt
		if due.Add(-before).After(now) || !due.After(now) {
			continue
		}
		res = append(res, o)
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].PaymentDueAt.Equal(*res[j].PaymentDueAt) {
			return res[i].PaymentDueAt.Before(*res[j].PaymentDueAt)
		}
		return res[i].ID < res[j].ID
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryOrderRepository) FindExpired(_ context.Context, now time.Time, limit int) ([]Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []Order
	for _, o := range r.orders {
		if o.Status != OrderStatusPending || o.PaymentMethod == codPaymentMethod || o.PaymentDueAt == nil ||
			!o.PaymentDueAt.Before(now) {
			continue
		}
		res = append(res, o)
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].PaymentDueAt.Equal(*res[j].PaymentDueAt) {
			return res[i].PaymentDueAt.Before(*res[j].PaymentDueAt)
		}
		return res[i].ID < res[j].ID
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryOrderRepository) MarkReminderSent(ctx context.Context, id string, now time.Time) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	if o.Status != OrderStatusPending || o.PaymentReminderSentAt != nil {
		return Order{}, ErrStatusConflict
	}
	sent := now
	o.PaymentReminderSentAt = &sent
	if r.reminderOutbox != nil && r.outbox != nil {
		row, err := r.reminderOutbox(o)
		if err != nil {
			return Order{}, fmt.Errorf("build payment reminder outbox row: %w", err)
		}
		if err := r.outbox.Enqueue(ctx, row); err != nil {
			return Order{}, err
		}
	}
	r.orders[id] = o
	return o, nil
}
