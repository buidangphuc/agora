package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresBundleRepository implements BundleRepository against a real pgxpool.
// Mirrors PostgresListingRepository: keyset pagination by id, listing_ids stored
// as a native text[] column.
type PostgresBundleRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresBundleRepository(pool *pgxpool.Pool) *PostgresBundleRepository {
	return &PostgresBundleRepository{pool: pool}
}

const bundleColumns = `id, seller_id, title, listing_ids, bundle_price, created_at`

func scanBundle(row pgx.Row, b *Bundle) error {
	return row.Scan(&b.ID, &b.SellerID, &b.Title, &b.ListingIDs, &b.BundlePrice, &b.CreatedAt)
}

// Create inserts a new bundle and returns the stored row.
func (r *PostgresBundleRepository) Create(ctx context.Context, b Bundle) (Bundle, error) {
	const q = `INSERT INTO bundles (id, seller_id, title, listing_ids, bundle_price)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + bundleColumns
	listingIDs := b.ListingIDs
	if listingIDs == nil {
		listingIDs = []string{}
	}
	var out Bundle
	row := r.pool.QueryRow(ctx, q, b.ID, b.SellerID, b.Title, listingIDs, b.BundlePrice)
	if err := scanBundle(row, &out); err != nil {
		return Bundle{}, fmt.Errorf("create bundle %q: %w", b.ID, err)
	}
	return out, nil
}

// Get loads a single bundle by id, mapping a miss to ErrBundleNotFound.
func (r *PostgresBundleRepository) Get(ctx context.Context, id string) (Bundle, error) {
	const q = `SELECT ` + bundleColumns + ` FROM bundles WHERE id = $1`
	var b Bundle
	if err := scanBundle(r.pool.QueryRow(ctx, q, id), &b); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bundle{}, ErrBundleNotFound
		}
		return Bundle{}, fmt.Errorf("get bundle %q: %w", id, err)
	}
	return b, nil
}

// ListBySeller returns a keyset page of the seller's bundles ordered by id.
func (r *PostgresBundleRepository) ListBySeller(ctx context.Context, sellerID, cursor string, pageSize int32) (BundlePage, error) {
	limit := clampPageSize(pageSize)
	const q = `SELECT ` + bundleColumns + `
		FROM bundles
		WHERE seller_id = $3 AND ($1 = '' OR id > $1)
		ORDER BY id
		LIMIT $2`
	rows, err := r.pool.Query(ctx, q, cursor, limit+1, sellerID)
	if err != nil {
		return BundlePage{}, fmt.Errorf("list bundles: %w", err)
	}
	defer rows.Close()

	items := make([]Bundle, 0, limit)
	for rows.Next() {
		var b Bundle
		if err := scanBundle(rows, &b); err != nil {
			return BundlePage{}, fmt.Errorf("scan bundle: %w", err)
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return BundlePage{}, fmt.Errorf("iterate bundles: %w", err)
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM bundles WHERE seller_id = $1`, sellerID).Scan(&total); err != nil {
		return BundlePage{}, fmt.Errorf("count bundles: %w", err)
	}
	return BundlePage{Items: items, NextCursor: next, Total: total}, nil
}

// compile-time assertion that the Postgres store satisfies the port.
var _ BundleRepository = (*PostgresBundleRepository)(nil)
