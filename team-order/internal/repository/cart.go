package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCartItemNotFound = errors.New("cart item not found")

type CartItem struct {
	ID          string
	UserID      string
	ListingID   string
	VariantID   string
	Quantity    int32
	UnitPrice   int64
	Title       string
	VariantName string
	ImageURL    string
	SellerID    string
}

type CartRepository interface {
	GetCart(ctx context.Context, userID string) ([]CartItem, error)
	AddItem(ctx context.Context, item CartItem) ([]CartItem, error)
	UpdateItem(ctx context.Context, userID, itemID string, quantity int32) ([]CartItem, error)
	RemoveItem(ctx context.Context, userID, itemID string) ([]CartItem, error)
	ClearCart(ctx context.Context, userID string) error
	RemoveItems(ctx context.Context, userID string, itemIDs []string) error
}

type PostgresCartRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCartRepository(pool *pgxpool.Pool) *PostgresCartRepository {
	return &PostgresCartRepository{pool: pool}
}

const cartColumns = `id, user_id, listing_id, variant_id, quantity, unit_price, title, variant_name, image_url, seller_id`

func scanCartItem(row pgx.Row, item *CartItem) error {
	return row.Scan(
		&item.ID,
		&item.UserID,
		&item.ListingID,
		&item.VariantID,
		&item.Quantity,
		&item.UnitPrice,
		&item.Title,
		&item.VariantName,
		&item.ImageURL,
		&item.SellerID,
	)
}

func (r *PostgresCartRepository) GetCart(ctx context.Context, userID string) ([]CartItem, error) {
	const q = `SELECT ` + cartColumns + ` FROM cart_items WHERE user_id = $1 ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}
	defer rows.Close()

	var items []CartItem
	for rows.Next() {
		var item CartItem
		if err := scanCartItem(rows, &item); err != nil {
			return nil, fmt.Errorf("scan cart item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresCartRepository) AddItem(ctx context.Context, item CartItem) ([]CartItem, error) {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.Quantity <= 0 {
		item.Quantity = 1
	}

	const q = `INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, variant_name, image_url, seller_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, listing_id, variant_id)
		DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, unit_price = EXCLUDED.unit_price, title = EXCLUDED.title, variant_name = EXCLUDED.variant_name, image_url = EXCLUDED.image_url, seller_id = EXCLUDED.seller_id, updated_at = now()`

	if _, err := r.pool.Exec(ctx, q, item.ID, item.UserID, item.ListingID, item.VariantID, item.Quantity, item.UnitPrice, item.Title, item.VariantName, item.ImageURL, item.SellerID); err != nil {
		return nil, fmt.Errorf("add cart item: %w", err)
	}

	return r.GetCart(ctx, item.UserID)
}

func (r *PostgresCartRepository) UpdateItem(ctx context.Context, userID, itemID string, quantity int32) ([]CartItem, error) {
	if quantity <= 0 {
		return r.RemoveItem(ctx, userID, itemID)
	}
	const q = `UPDATE cart_items SET quantity = $3, updated_at = now() WHERE id = $1 AND user_id = $2`
	res, err := r.pool.Exec(ctx, q, itemID, userID, quantity)
	if err != nil {
		return nil, fmt.Errorf("update cart item: %w", err)
	}
	if res.RowsAffected() == 0 {
		return nil, ErrCartItemNotFound
	}
	return r.GetCart(ctx, userID)
}

func (r *PostgresCartRepository) RemoveItem(ctx context.Context, userID, itemID string) ([]CartItem, error) {
	const q = `DELETE FROM cart_items WHERE id = $1 AND user_id = $2`
	res, err := r.pool.Exec(ctx, q, itemID, userID)
	if err != nil {
		return nil, fmt.Errorf("remove cart item: %w", err)
	}
	if res.RowsAffected() == 0 {
		return nil, ErrCartItemNotFound
	}
	return r.GetCart(ctx, userID)
}

func (r *PostgresCartRepository) ClearCart(ctx context.Context, userID string) error {
	const q = `DELETE FROM cart_items WHERE user_id = $1`
	_, err := r.pool.Exec(ctx, q, userID)
	return err
}

func (r *PostgresCartRepository) RemoveItems(ctx context.Context, userID string, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	const q = `DELETE FROM cart_items WHERE user_id = $1 AND id = ANY($2)`
	_, err := r.pool.Exec(ctx, q, userID, itemIDs)
	return err
}

// InMemoryCartRepository is for unit tests.
type InMemoryCartRepository struct {
	mu    sync.RWMutex
	items map[string]CartItem
}

func NewInMemoryCartRepository() *InMemoryCartRepository {
	return &InMemoryCartRepository{items: make(map[string]CartItem)}
}

func (r *InMemoryCartRepository) GetCart(_ context.Context, userID string) ([]CartItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []CartItem
	for _, item := range r.items {
		if item.UserID == userID {
			res = append(res, item)
		}
	}
	return res, nil
}

func (r *InMemoryCartRepository) AddItem(ctx context.Context, item CartItem) ([]CartItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	// check existing
	found := false
	for id, existing := range r.items {
		if existing.UserID == item.UserID && existing.ListingID == item.ListingID && existing.VariantID == item.VariantID {
			existing.Quantity += item.Quantity
			existing.UnitPrice = item.UnitPrice
			existing.Title = item.Title
			existing.VariantName = item.VariantName
			existing.ImageURL = item.ImageURL
			existing.SellerID = item.SellerID
			r.items[id] = existing
			found = true
			break
		}
	}
	if !found {
		r.items[item.ID] = item
	}
	r.mu.Unlock()
	res, err := r.GetCart(ctx, item.UserID)
	r.mu.Lock()
	return res, err
}

func (r *InMemoryCartRepository) UpdateItem(ctx context.Context, userID, itemID string, quantity int32) ([]CartItem, error) {
	r.mu.Lock()
	item, ok := r.items[itemID]
	if !ok || item.UserID != userID {
		r.mu.Unlock()
		return nil, ErrCartItemNotFound
	}
	if quantity <= 0 {
		delete(r.items, itemID)
	} else {
		item.Quantity = quantity
		r.items[itemID] = item
	}
	r.mu.Unlock()
	return r.GetCart(ctx, userID)
}

func (r *InMemoryCartRepository) RemoveItem(ctx context.Context, userID, itemID string) ([]CartItem, error) {
	r.mu.Lock()
	item, ok := r.items[itemID]
	if !ok || item.UserID != userID {
		r.mu.Unlock()
		return nil, ErrCartItemNotFound
	}
	delete(r.items, itemID)
	r.mu.Unlock()
	return r.GetCart(ctx, userID)
}

func (r *InMemoryCartRepository) ClearCart(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, item := range r.items {
		if item.UserID == userID {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *InMemoryCartRepository) RemoveItems(_ context.Context, userID string, itemIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	idSet := make(map[string]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		idSet[id] = struct{}{}
	}
	for id, item := range r.items {
		if item.UserID == userID {
			if _, match := idSet[id]; match {
				delete(r.items, id)
			}
		}
	}
	return nil
}
