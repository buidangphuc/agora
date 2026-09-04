package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStorefrontRepository implements StorefrontRepository against pgxpool.
// The presentational fields live in a JSONB `config` column; seller_id and slug
// are their own columns (primary key / unique index).
type PostgresStorefrontRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresStorefrontRepository(pool *pgxpool.Pool) *PostgresStorefrontRepository {
	return &PostgresStorefrontRepository{pool: pool}
}

// storefrontConfig is the JSON shape stored in the storefronts.config column.
type storefrontConfig struct {
	BannerURL          string   `json:"banner_url,omitempty"`
	Tagline            string   `json:"tagline,omitempty"`
	FeaturedListingIDs []string `json:"featured_listing_ids,omitempty"`
	Theme              string   `json:"theme,omitempty"`
}

func encodeConfig(s Storefront) ([]byte, error) {
	return json.Marshal(storefrontConfig{
		BannerURL:          s.BannerURL,
		Tagline:            s.Tagline,
		FeaturedListingIDs: s.FeaturedListingIDs,
		Theme:              s.Theme,
	})
}

// scanStorefront reads a (seller_id, slug, config, updated_at) row into s.
func scanStorefront(row pgx.Row, s *Storefront) error {
	var raw []byte
	if err := row.Scan(&s.SellerID, &s.Slug, &raw, &s.UpdatedAt); err != nil {
		return err
	}
	var cfg storefrontConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("decode storefront config for %q: %w", s.SellerID, err)
		}
	}
	s.BannerURL = cfg.BannerURL
	s.Tagline = cfg.Tagline
	s.FeaturedListingIDs = cfg.FeaturedListingIDs
	s.Theme = cfg.Theme
	return nil
}

const storefrontColumns = `seller_id, slug, config, updated_at`

// Upsert inserts or replaces the seller's storefront. A slug already owned by a
// different seller trips the unique index and maps to ErrSlugTaken.
func (r *PostgresStorefrontRepository) Upsert(ctx context.Context, s Storefront) (Storefront, error) {
	cfg, err := encodeConfig(s)
	if err != nil {
		return Storefront{}, fmt.Errorf("encode storefront config: %w", err)
	}
	const q = `INSERT INTO storefronts (seller_id, slug, config, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (seller_id) DO UPDATE
		SET slug = EXCLUDED.slug, config = EXCLUDED.config, updated_at = now()
		RETURNING ` + storefrontColumns
	var out Storefront
	if err := scanStorefront(r.pool.QueryRow(ctx, q, s.SellerID, s.Slug, cfg), &out); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Storefront{}, ErrSlugTaken
		}
		return Storefront{}, fmt.Errorf("upsert storefront %q: %w", s.SellerID, err)
	}
	return out, nil
}

// GetBySeller loads a storefront by owner id, or ErrStorefrontNotFound.
func (r *PostgresStorefrontRepository) GetBySeller(ctx context.Context, sellerID string) (Storefront, error) {
	const q = `SELECT ` + storefrontColumns + ` FROM storefronts WHERE seller_id = $1`
	return r.getOne(ctx, q, sellerID)
}

// GetBySlug loads a storefront by its public slug, or ErrStorefrontNotFound.
func (r *PostgresStorefrontRepository) GetBySlug(ctx context.Context, slug string) (Storefront, error) {
	const q = `SELECT ` + storefrontColumns + ` FROM storefronts WHERE slug = $1`
	return r.getOne(ctx, q, slug)
}

func (r *PostgresStorefrontRepository) getOne(ctx context.Context, q, arg string) (Storefront, error) {
	var s Storefront
	if err := scanStorefront(r.pool.QueryRow(ctx, q, arg), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Storefront{}, ErrStorefrontNotFound
		}
		return Storefront{}, fmt.Errorf("get storefront: %w", err)
	}
	return s, nil
}

// compile-time assertion that the Postgres store satisfies the port.
var _ StorefrontRepository = (*PostgresStorefrontRepository)(nil)
