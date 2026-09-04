package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCampaignNotFound is returned when a flash-sale campaign lookup misses.
var ErrCampaignNotFound = errors.New("flash sale campaign not found")

// FlashSaleCampaign is the persistence-layer view of a campaign. Field semantics
// mirror platform.promotion.v1.FlashSaleCampaign; wire mapping lives in the
// service layer.
type FlashSaleCampaign struct {
	ID        string
	ListingID string
	VariantID string
	SalePrice int64
	StockCap  int64
	StockSold int64
	StartsAt  time.Time
	EndsAt    time.Time
}

// FlashSaleRepository is the persistence seam for flash-sale campaigns.
type FlashSaleRepository interface {
	// Create inserts a campaign; a blank ID is filled with a generated one.
	Create(ctx context.Context, c FlashSaleCampaign) (FlashSaleCampaign, error)
	// GetByID fetches a campaign by id (ErrCampaignNotFound on miss).
	GetByID(ctx context.Context, id string) (FlashSaleCampaign, error)
	// GetActiveByListing returns the campaign for a listing whose [starts_at,
	// ends_at] window contains now; ErrCampaignNotFound when none is active.
	GetActiveByListing(ctx context.Context, listingID string, now time.Time) (FlashSaleCampaign, error)
	// ListActive returns campaigns active at now, bounded by limit/offset.
	ListActive(ctx context.Context, now time.Time, limit, offset int) ([]FlashSaleCampaign, error)
	// Ping verifies the backing store is reachable.
	Ping(ctx context.Context) error
}

// ── Postgres implementation ──

// PostgresFlashSaleRepository persists campaigns in promotion_db.
type PostgresFlashSaleRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresFlashSaleRepository(pool *pgxpool.Pool) *PostgresFlashSaleRepository {
	return &PostgresFlashSaleRepository{pool: pool}
}

const campaignColumns = `id, listing_id, variant_id, sale_price, stock_cap, stock_sold, starts_at, ends_at`

func scanCampaign(row pgx.Row, c *FlashSaleCampaign) error {
	return row.Scan(&c.ID, &c.ListingID, &c.VariantID, &c.SalePrice, &c.StockCap, &c.StockSold, &c.StartsAt, &c.EndsAt)
}

func (r *PostgresFlashSaleRepository) Create(ctx context.Context, c FlashSaleCampaign) (FlashSaleCampaign, error) {
	if c.ID == "" {
		c.ID = newID()
	}
	const q = `INSERT INTO flash_sale_campaigns (` + campaignColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	if _, err := r.pool.Exec(ctx, q,
		c.ID, c.ListingID, c.VariantID, c.SalePrice, c.StockCap, c.StockSold, nullableTime(c.StartsAt), nullableTime(c.EndsAt),
	); err != nil {
		return FlashSaleCampaign{}, fmt.Errorf("insert campaign: %w", err)
	}
	return c, nil
}

func (r *PostgresFlashSaleRepository) GetByID(ctx context.Context, id string) (FlashSaleCampaign, error) {
	const q = `SELECT ` + campaignColumns + ` FROM flash_sale_campaigns WHERE id = $1`
	var c FlashSaleCampaign
	if err := scanCampaign(r.pool.QueryRow(ctx, q, id), &c); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlashSaleCampaign{}, ErrCampaignNotFound
		}
		return FlashSaleCampaign{}, fmt.Errorf("get campaign %q: %w", id, err)
	}
	return c, nil
}

func (r *PostgresFlashSaleRepository) GetActiveByListing(ctx context.Context, listingID string, now time.Time) (FlashSaleCampaign, error) {
	const q = `SELECT ` + campaignColumns + ` FROM flash_sale_campaigns
		WHERE listing_id = $1
		  AND (starts_at IS NULL OR starts_at <= $2)
		  AND (ends_at IS NULL OR ends_at > $2)
		ORDER BY starts_at DESC NULLS LAST
		LIMIT 1`
	var c FlashSaleCampaign
	if err := scanCampaign(r.pool.QueryRow(ctx, q, listingID, now), &c); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlashSaleCampaign{}, ErrCampaignNotFound
		}
		return FlashSaleCampaign{}, fmt.Errorf("get active campaign for listing %q: %w", listingID, err)
	}
	return c, nil
}

func (r *PostgresFlashSaleRepository) ListActive(ctx context.Context, now time.Time, limit, offset int) ([]FlashSaleCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT ` + campaignColumns + ` FROM flash_sale_campaigns
		WHERE (starts_at IS NULL OR starts_at <= $1)
		  AND (ends_at IS NULL OR ends_at > $1)
		ORDER BY starts_at DESC NULLS LAST
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, now, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	defer rows.Close()
	var out []FlashSaleCampaign
	for rows.Next() {
		var c FlashSaleCampaign
		if err := scanCampaign(rows, &c); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresFlashSaleRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// ── In-memory implementation ──

// InMemoryFlashSaleRepository is the DB-less backend used when DATABASE_ENABLED
// is false and in unit tests.
type InMemoryFlashSaleRepository struct {
	mu        sync.RWMutex
	campaigns map[string]FlashSaleCampaign
}

func NewInMemoryFlashSaleRepository() *InMemoryFlashSaleRepository {
	return &InMemoryFlashSaleRepository{campaigns: make(map[string]FlashSaleCampaign)}
}

// active reports whether now falls within the campaign's [starts_at, ends_at)
// window; a zero bound is treated as open-ended.
func active(c FlashSaleCampaign, now time.Time) bool {
	if !c.StartsAt.IsZero() && now.Before(c.StartsAt) {
		return false
	}
	if !c.EndsAt.IsZero() && !now.Before(c.EndsAt) {
		return false
	}
	return true
}

func (r *InMemoryFlashSaleRepository) Create(_ context.Context, c FlashSaleCampaign) (FlashSaleCampaign, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = newID()
	}
	r.campaigns[c.ID] = c
	return c, nil
}

func (r *InMemoryFlashSaleRepository) GetByID(_ context.Context, id string) (FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.campaigns[id]
	if !ok {
		return FlashSaleCampaign{}, ErrCampaignNotFound
	}
	return c, nil
}

func (r *InMemoryFlashSaleRepository) GetActiveByListing(_ context.Context, listingID string, now time.Time) (FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found []FlashSaleCampaign
	for _, c := range r.campaigns {
		if c.ListingID == listingID && active(c, now) {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		return FlashSaleCampaign{}, ErrCampaignNotFound
	}
	// Prefer the most recently started campaign, mirroring the Postgres ordering.
	sort.Slice(found, func(i, j int) bool { return found[i].StartsAt.After(found[j].StartsAt) })
	return found[0], nil
}

func (r *InMemoryFlashSaleRepository) ListActive(_ context.Context, now time.Time, limit, offset int) ([]FlashSaleCampaign, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []FlashSaleCampaign
	for _, c := range r.campaigns {
		if active(c, now) {
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].StartsAt.After(all[j].StartsAt) })
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *InMemoryFlashSaleRepository) Ping(_ context.Context) error { return nil }
