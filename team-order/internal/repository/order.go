package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrderNotFound = errors.New("order not found")

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
}

type OrderRepository interface {
	CreateOrder(ctx context.Context, order Order) (Order, error)
	GetOrder(ctx context.Context, id string) (Order, error)
	ListBuyerOrders(ctx context.Context, buyerID string, statusFilter int32) ([]Order, error)
	ListSellerOrders(ctx context.Context, sellerID string, statusFilter int32) ([]Order, error)
	UpdateOrderStatus(ctx context.Context, id string, status OrderStatus, trackingNumber string) (Order, error)
}

type PostgresOrderRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresOrderRepository(pool *pgxpool.Pool) *PostgresOrderRepository {
	return &PostgresOrderRepository{pool: pool}
}

const orderColumns = `id, buyer_id, seller_id, status, total_amount, currency, shipping_address, tracking_number, created_at, updated_at, shipping_fee, items_subtotal, payment_method, voucher_code, discount_amount`
const orderItemColumns = `id, order_id, listing_id, variant_id, title, variant_name, quantity, unit_price, image_url`

func scanOrder(row pgx.Row, o *Order) error {
	var addrRaw []byte
	var statusInt int32
	if err := row.Scan(&o.ID, &o.BuyerID, &o.SellerID, &statusInt, &o.TotalAmount, &o.Currency, &addrRaw, &o.TrackingNumber, &o.CreatedAt, &o.UpdatedAt, &o.ShippingFee, &o.ItemsSubtotal, &o.PaymentMethod, &o.VoucherCode, &o.DiscountAmount); err != nil {
		return err
	}
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

	addrBytes, err := json.Marshal(order.ShippingAddress)
	if err != nil {
		return Order{}, fmt.Errorf("marshal address: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const qOrder = `INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, currency, shipping_address, tracking_number, created_at, updated_at, shipping_fee, items_subtotal, payment_method, voucher_code, discount_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	if _, err := tx.Exec(ctx, qOrder, order.ID, order.BuyerID, order.SellerID, int32(order.Status), order.TotalAmount, order.Currency, addrBytes, order.TrackingNumber, order.CreatedAt, order.UpdatedAt, order.ShippingFee, order.ItemsSubtotal, order.PaymentMethod, order.VoucherCode, order.DiscountAmount); err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
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
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}
	return order, nil
}

func (r *PostgresOrderRepository) loadItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	const q = `SELECT ` + orderItemColumns + ` FROM order_items WHERE order_id = $1`
	rows, err := r.pool.Query(ctx, q, orderID)
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

func (r *PostgresOrderRepository) UpdateOrderStatus(ctx context.Context, id string, status OrderStatus, trackingNumber string) (Order, error) {
	var q string
	var args []any
	if trackingNumber != "" {
		q = `UPDATE orders SET status = $2, tracking_number = $3, updated_at = now() WHERE id = $1 RETURNING ` + orderColumns
		args = []any{id, int32(status), trackingNumber}
	} else {
		q = `UPDATE orders SET status = $2, updated_at = now() WHERE id = $1 RETURNING ` + orderColumns
		args = []any{id, int32(status)}
	}

	var o Order
	if err := scanOrder(r.pool.QueryRow(ctx, q, args...), &o); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("update order status: %w", err)
	}
	items, err := r.loadItems(ctx, o.ID)
	if err != nil {
		return Order{}, err
	}
	o.Items = items
	return o, nil
}

// InMemoryOrderRepository for unit tests
type InMemoryOrderRepository struct {
	mu     sync.RWMutex
	orders map[string]Order
}

func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{orders: make(map[string]Order)}
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

func (r *InMemoryOrderRepository) UpdateOrderStatus(_ context.Context, id string, status OrderStatus, trackingNumber string) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	o.Status = status
	if trackingNumber != "" {
		o.TrackingNumber = trackingNumber
	}
	o.UpdatedAt = time.Now()
	r.orders[id] = o
	return o, nil
}
