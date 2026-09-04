package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the subset of pgx used by the write path. Both *pgxpool.Pool and
// pgx.Tx satisfy it, so a repository write can run either directly on the pool
// (the plain methods) or inside a caller-owned transaction (the *Tx free
// functions), which is what the transactional outbox relies on: the listing
// write and the outbox INSERT share one pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresListingRepository implements ListingRepository against a real pgxpool.
type PostgresListingRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresListingRepository(pool *pgxpool.Pool) *PostgresListingRepository {
	return &PostgresListingRepository{pool: pool}
}

const listingColumns = `id, title, description, price, currency, status, seller_id, image_keys, category_id, stock`

func scanListing(row pgx.Row, l *Listing) error {
	return row.Scan(&l.ID, &l.Title, &l.Description, &l.Price, &l.Currency, &l.Status, &l.SellerID, &l.ImageKeys, &l.CategoryID, &l.Stock)
}

// Get loads a single listing by id and its variants, mapping a miss to ErrNotFound.
func (r *PostgresListingRepository) Get(ctx context.Context, id string) (Listing, error) {
	const q = `SELECT ` + listingColumns + ` FROM listings WHERE id = $1`
	var l Listing
	if err := scanListing(r.pool.QueryRow(ctx, q, id), &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Listing{}, ErrNotFound
		}
		return Listing{}, fmt.Errorf("get listing %q: %w", id, err)
	}
	variants, err := loadVariants(ctx, r.pool, id)
	if err != nil {
		return Listing{}, err
	}
	l.Variants = variants
	return l, nil
}

// loadVariants reads a listing's variants over any DBTX (pool or tx).
func loadVariants(ctx context.Context, q DBTX, listingID string) ([]Variant, error) {
	const sql = `SELECT id, listing_id, name, sku, price, stock, image_url FROM listing_variants WHERE listing_id = $1 ORDER BY id`
	rows, err := q.Query(ctx, sql, listingID)
	if err != nil {
		return nil, fmt.Errorf("load variants for %q: %w", listingID, err)
	}
	defer rows.Close()

	var items []Variant
	for rows.Next() {
		var v Variant
		if err := rows.Scan(&v.ID, &v.ListingID, &v.Name, &v.SKU, &v.Price, &v.Stock, &v.ImageURL); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

// List returns a keyset page ordered by id, optionally filtered by status.
func (r *PostgresListingRepository) List(ctx context.Context, cursor string, pageSize int32, status string) (Page, error) {
	limit := clampPageSize(pageSize)
	const q = `SELECT ` + listingColumns + `
		FROM listings
		WHERE ($1 = '' OR id > $1) AND ($3 = '' OR status = $3)
		ORDER BY id
		LIMIT $2`
	items, next, err := r.queryPage(ctx, q, cursor, limit, status)
	if err != nil {
		return Page{}, err
	}
	total, err := r.count(ctx, `SELECT count(*) FROM listings WHERE ($1 = '' OR status = $1)`, status)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, NextCursor: next, Total: total}, nil
}

// ListByOwner returns a keyset page of the owner's listings.
func (r *PostgresListingRepository) ListByOwner(ctx context.Context, ownerID, cursor string, pageSize int32) (Page, error) {
	limit := clampPageSize(pageSize)
	const q = `SELECT ` + listingColumns + `
		FROM listings
		WHERE seller_id = $3 AND ($1 = '' OR id > $1)
		ORDER BY id
		LIMIT $2`
	items, next, err := r.queryPage(ctx, q, cursor, limit, ownerID)
	if err != nil {
		return Page{}, err
	}
	total, err := r.count(ctx, `SELECT count(*) FROM listings WHERE seller_id = $1`, ownerID)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, NextCursor: next, Total: total}, nil
}

// queryPage runs a keyset query that takes (cursor, limit+1, arg) and returns the
// page items (dropping the probe row) + next cursor.
func (r *PostgresListingRepository) queryPage(ctx context.Context, q, cursor string, limit int, arg string) ([]Listing, string, error) {
	rows, err := r.pool.Query(ctx, q, cursor, limit+1, arg)
	if err != nil {
		return nil, "", fmt.Errorf("list listings: %w", err)
	}
	defer rows.Close()

	items := make([]Listing, 0, limit)
	for rows.Next() {
		var l Listing
		if err := scanListing(rows, &l); err != nil {
			return nil, "", fmt.Errorf("scan listing: %w", err)
		}
		items = append(items, l)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate listings: %w", err)
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

// Create inserts a new listing and its variants, returning the stored row.
func (r *PostgresListingRepository) Create(ctx context.Context, l Listing) (Listing, error) {
	return createListing(ctx, r.pool, l)
}

// createListing inserts a listing + variants over any DBTX (pool or tx).
func createListing(ctx context.Context, q DBTX, l Listing) (Listing, error) {
	const sql = `INSERT INTO listings (id, title, description, price, currency, status, seller_id, image_keys, category_id, stock)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ` + listingColumns
	var out Listing
	imageKeys := l.ImageKeys
	if imageKeys == nil {
		imageKeys = []string{}
	}
	row := q.QueryRow(ctx, sql, l.ID, l.Title, l.Description, l.Price, l.Currency, l.Status, l.SellerID, imageKeys, l.CategoryID, l.Stock)
	if err := scanListing(row, &out); err != nil {
		return Listing{}, fmt.Errorf("create listing %q: %w", l.ID, err)
	}

	// Persist variants if any
	for _, v := range l.Variants {
		vID := v.ID
		if vID == "" {
			vID = uuid.NewString()
		}
		const vq = `INSERT INTO listing_variants (id, listing_id, name, sku, price, stock, image_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`
		if _, err := q.Exec(ctx, vq, vID, l.ID, v.Name, v.SKU, v.Price, v.Stock, v.ImageURL); err != nil {
			return Listing{}, fmt.Errorf("create variant for listing %q: %w", l.ID, err)
		}
	}
	variants, err := loadVariants(ctx, q, l.ID)
	if err != nil {
		return Listing{}, err
	}
	out.Variants = variants
	return out, nil
}

// Update replaces an existing listing and its variants.
func (r *PostgresListingRepository) Update(ctx context.Context, l Listing) (Listing, error) {
	return updateListing(ctx, r.pool, l)
}

// updateListing replaces a listing + variants over any DBTX (pool or tx).
func updateListing(ctx context.Context, q DBTX, l Listing) (Listing, error) {
	const sql = `UPDATE listings
		SET title = $2, description = $3, price = $4, currency = $5, status = $6, seller_id = $7, image_keys = $8, category_id = $9, stock = $10, updated_at = now()
		WHERE id = $1
		RETURNING ` + listingColumns
	var out Listing
	imageKeys := l.ImageKeys
	if imageKeys == nil {
		imageKeys = []string{}
	}
	row := q.QueryRow(ctx, sql, l.ID, l.Title, l.Description, l.Price, l.Currency, l.Status, l.SellerID, imageKeys, l.CategoryID, l.Stock)
	if err := scanListing(row, &out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Listing{}, ErrNotFound
		}
		return Listing{}, fmt.Errorf("update listing %q: %w", l.ID, err)
	}

	// Replace variants
	if _, err := q.Exec(ctx, `DELETE FROM listing_variants WHERE listing_id = $1`, l.ID); err != nil {
		return Listing{}, fmt.Errorf("delete old variants for %q: %w", l.ID, err)
	}
	for _, v := range l.Variants {
		vID := v.ID
		if vID == "" {
			vID = uuid.NewString()
		}
		const vq = `INSERT INTO listing_variants (id, listing_id, name, sku, price, stock, image_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`
		if _, err := q.Exec(ctx, vq, vID, l.ID, v.Name, v.SKU, v.Price, v.Stock, v.ImageURL); err != nil {
			return Listing{}, fmt.Errorf("insert variant for %q: %w", l.ID, err)
		}
	}

	variants, err := loadVariants(ctx, q, l.ID)
	if err != nil {
		return Listing{}, err
	}
	out.Variants = variants
	return out, nil
}

// Delete removes a listing, returning the deleted row or ErrNotFound.
func (r *PostgresListingRepository) Delete(ctx context.Context, id string) (Listing, error) {
	return deleteListing(ctx, r.pool, id)
}

// deleteListing removes a listing over any DBTX (pool or tx).
func deleteListing(ctx context.Context, q DBTX, id string) (Listing, error) {
	const sql = `DELETE FROM listings WHERE id = $1 RETURNING ` + listingColumns
	var out Listing
	if err := scanListing(q.QueryRow(ctx, sql, id), &out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Listing{}, ErrNotFound
		}
		return Listing{}, fmt.Errorf("delete listing %q: %w", id, err)
	}
	return out, nil
}

// ReserveStock atomically decrements inventory if sufficient stock is available.
func (r *PostgresListingRepository) ReserveStock(ctx context.Context, listingID, variantID string, quantity int32) error {
	if quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if variantID == "" {
		const q = `UPDATE listings SET stock = stock - $1 WHERE id = $2 AND stock >= $1`
		res, err := r.pool.Exec(ctx, q, quantity, listingID)
		if err != nil {
			return fmt.Errorf("reserve base stock: %w", err)
		}
		if res.RowsAffected() == 0 {
			var exists bool
			_ = r.pool.QueryRow(ctx, `SELECT true FROM listings WHERE id = $1`, listingID).Scan(&exists)
			if !exists {
				return ErrNotFound
			}
			return ErrOutOfStock
		}
		return nil
	}

	const q = `UPDATE listing_variants SET stock = stock - $1 WHERE id = $2 AND listing_id = $3 AND stock >= $1`
	res, err := r.pool.Exec(ctx, q, quantity, variantID, listingID)
	if err != nil {
		return fmt.Errorf("reserve variant stock: %w", err)
	}
	if res.RowsAffected() == 0 {
		var exists bool
		_ = r.pool.QueryRow(ctx, `SELECT true FROM listing_variants WHERE id = $1 AND listing_id = $2`, variantID, listingID).Scan(&exists)
		if !exists {
			return ErrVariantNotFound
		}
		return ErrOutOfStock
	}
	return nil
}

// ReserveStockIdempotent decrements stock and records a reservation keyed on
// reservationID, in ONE transaction so the two commit together (AD5). If the
// reservation_id was already applied, it is a no-op returning nil — so a retried
// checkout never double-decrements. An empty reservationID falls back to a plain
// (non-idempotent) reserve.
func (r *PostgresListingRepository) ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32, expiresAt time.Time) error {
	if quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if reservationID == "" {
		return r.ReserveStock(ctx, listingID, variantID, quantity)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reserve tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Idempotency guard: a reservation already applied for this id is a no-op.
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reservations WHERE reservation_id = $1)`, reservationID).Scan(&exists); err != nil {
		return fmt.Errorf("lookup reservation %q: %w", reservationID, err)
	}
	if exists {
		return nil
	}

	if variantID == "" {
		const q = `UPDATE listings SET stock = stock - $1 WHERE id = $2 AND stock >= $1`
		res, err := tx.Exec(ctx, q, quantity, listingID)
		if err != nil {
			return fmt.Errorf("reserve base stock: %w", err)
		}
		if res.RowsAffected() == 0 {
			var ex bool
			_ = tx.QueryRow(ctx, `SELECT true FROM listings WHERE id = $1`, listingID).Scan(&ex)
			if !ex {
				return ErrNotFound
			}
			return ErrOutOfStock
		}
	} else {
		const q = `UPDATE listing_variants SET stock = stock - $1 WHERE id = $2 AND listing_id = $3 AND stock >= $1`
		res, err := tx.Exec(ctx, q, quantity, variantID, listingID)
		if err != nil {
			return fmt.Errorf("reserve variant stock: %w", err)
		}
		if res.RowsAffected() == 0 {
			var ex bool
			_ = tx.QueryRow(ctx, `SELECT true FROM listing_variants WHERE id = $1 AND listing_id = $2`, variantID, listingID).Scan(&ex)
			if !ex {
				return ErrVariantNotFound
			}
			return ErrOutOfStock
		}
	}

	const ins = `INSERT INTO reservations (reservation_id, listing_id, variant_id, quantity, status, expires_at)
		VALUES ($1, $2, $3, $4, 'active', $5)`
	if _, err := tx.Exec(ctx, ins, reservationID, listingID, variantID, quantity, expiresAt); err != nil {
		return fmt.Errorf("insert reservation %q: %w", reservationID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reserve tx: %w", err)
	}
	return nil
}

// SweepExpiredReservations restores stock for every active reservation past its
// TTL and marks it released, in one transaction (AD3 domain side). Rows are
// leased with FOR UPDATE SKIP LOCKED so concurrent sweepers never double-restore.
func (r *PostgresListingRepository) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin sweep tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const sel = `SELECT reservation_id, listing_id, variant_id, quantity
		FROM reservations
		WHERE status = 'active' AND expires_at <= $1
		FOR UPDATE SKIP LOCKED`
	rows, err := tx.Query(ctx, sel, now)
	if err != nil {
		return 0, fmt.Errorf("select expired reservations: %w", err)
	}
	type expired struct {
		id, listingID, variantID string
		quantity                 int32
	}
	var batch []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.id, &e.listingID, &e.variantID, &e.quantity); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired reservation: %w", err)
		}
		batch = append(batch, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate expired reservations: %w", err)
	}

	for _, e := range batch {
		if e.variantID == "" {
			if _, err := tx.Exec(ctx, `UPDATE listings SET stock = stock + $1 WHERE id = $2`, e.quantity, e.listingID); err != nil {
				return 0, fmt.Errorf("restore base stock: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE listing_variants SET stock = stock + $1 WHERE id = $2 AND listing_id = $3`, e.quantity, e.variantID, e.listingID); err != nil {
				return 0, fmt.Errorf("restore variant stock: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE reservations SET status = 'released', released_at = now() WHERE reservation_id = $1`, e.id); err != nil {
			return 0, fmt.Errorf("mark reservation %q released: %w", e.id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit sweep tx: %w", err)
	}
	return len(batch), nil
}

// ReleaseStock releases previously reserved inventory back into stock.
func (r *PostgresListingRepository) ReleaseStock(ctx context.Context, listingID, variantID string, quantity int32) error {
	if quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if variantID == "" {
		const q = `UPDATE listings SET stock = stock + $1 WHERE id = $2`
		res, err := r.pool.Exec(ctx, q, quantity, listingID)
		if err != nil {
			return fmt.Errorf("release base stock: %w", err)
		}
		if res.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}

	const q = `UPDATE listing_variants SET stock = stock + $1 WHERE id = $2 AND listing_id = $3`
	res, err := r.pool.Exec(ctx, q, quantity, variantID, listingID)
	if err != nil {
		return fmt.Errorf("release variant stock: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrVariantNotFound
	}
	return nil
}

func (r *PostgresListingRepository) count(ctx context.Context, q, arg string) (int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, q, arg).Scan(&total); err != nil {
		return 0, fmt.Errorf("count listings: %w", err)
	}
	return total, nil
}

// compile-time assertion that the Postgres store satisfies the port.
var _ ListingRepository = (*PostgresListingRepository)(nil)
