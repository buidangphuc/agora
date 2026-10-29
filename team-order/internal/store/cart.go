package store

import (
	"context"
	"errors"
	"fmt"

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
	// UpdateItemPrice sets only unit_price of the user's line; ErrCartItemNotFound if none.
	UpdateItemPrice(ctx context.Context, userID, itemID string, unitPrice int64) error
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

func (r *PostgresCartRepository) UpdateItemPrice(ctx context.Context, userID, itemID string, unitPrice int64) error {
	const q = `UPDATE cart_items SET unit_price = $3, updated_at = now() WHERE id = $1 AND user_id = $2`
	res, err := r.pool.Exec(ctx, q, itemID, userID, unitPrice)
	if err != nil {
		return fmt.Errorf("update cart item price: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrCartItemNotFound
	}
	return nil
}
