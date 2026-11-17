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
	ID             string
	UserID         string
	ListingID      string
	VariantID      string
	Quantity       int32
	UnitPrice      int64
	Title          string
	VariantName    string
	ImageURL       string
	SellerID       string
	BundleID       string
	BundleDiscount int64
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

const cartColumns = `id, user_id, listing_id, variant_id, quantity, unit_price, title, variant_name, image_url, seller_id, bundle_id, bundle_discount`

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
		&item.BundleID,
		&item.BundleDiscount,
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
		ON CONFLICT (user_id, listing_id, variant_id, bundle_id)
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

func (r *PostgresCartRepository) AddBundleLines(ctx context.Context, userID string, lines []CartItem) ([]CartItem, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("add bundle lines: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `INSERT INTO cart_items (id, user_id, listing_id, variant_id, quantity, unit_price, title, variant_name, image_url, seller_id, bundle_id, bundle_discount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (user_id, listing_id, variant_id, bundle_id)
		DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, unit_price = EXCLUDED.unit_price, title = EXCLUDED.title, variant_name = EXCLUDED.variant_name, image_url = EXCLUDED.image_url, seller_id = EXCLUDED.seller_id, bundle_discount = EXCLUDED.bundle_discount, updated_at = now()`
	for _, l := range lines {
		if l.Quantity <= 0 {
			l.Quantity = 1
		}
		if _, err := tx.Exec(ctx, q, uuid.NewString(), userID, l.ListingID, l.VariantID, l.Quantity, l.UnitPrice, l.Title, l.VariantName, l.ImageURL, l.SellerID, l.BundleID, l.BundleDiscount); err != nil {
			return nil, fmt.Errorf("add bundle line: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("add bundle lines commit: %w", err)
	}
	return r.GetCart(ctx, userID)
}

func (r *PostgresCartRepository) DropBundleTag(ctx context.Context, userID, bundleID string) ([]CartItem, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("drop bundle tag: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const merge = `UPDATE cart_items p SET quantity = p.quantity + b.quantity, updated_at = now()
		FROM cart_items b
		WHERE b.user_id = $1 AND b.bundle_id = $2
		  AND p.user_id = b.user_id AND p.listing_id = b.listing_id AND p.variant_id = b.variant_id AND p.bundle_id = ''`
	if _, err := tx.Exec(ctx, merge, userID, bundleID); err != nil {
		return nil, fmt.Errorf("drop bundle tag merge: %w", err)
	}
	const del = `DELETE FROM cart_items b USING cart_items p
		WHERE b.user_id = $1 AND b.bundle_id = $2
		  AND p.user_id = b.user_id AND p.listing_id = b.listing_id AND p.variant_id = b.variant_id AND p.bundle_id = ''`
	if _, err := tx.Exec(ctx, del, userID, bundleID); err != nil {
		return nil, fmt.Errorf("drop bundle tag delete: %w", err)
	}
	const clr = `UPDATE cart_items SET bundle_id = '', bundle_discount = 0, updated_at = now() WHERE user_id = $1 AND bundle_id = $2`
	if _, err := tx.Exec(ctx, clr, userID, bundleID); err != nil {
		return nil, fmt.Errorf("drop bundle tag clear: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("drop bundle tag commit: %w", err)
	}
	return r.GetCart(ctx, userID)
}
