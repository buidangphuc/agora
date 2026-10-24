package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
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
	// Create inserts a campaign and its outbox row in one transaction. A blank ID is
	// filled with a generated one before event is called with the stored campaign.
	Create(ctx context.Context, c FlashSaleCampaign, event func(FlashSaleCampaign) (outbox.Row, error)) (FlashSaleCampaign, error)
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
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewPostgresFlashSaleRepository(pool *pgxpool.Pool, ob *outbox.Store) *PostgresFlashSaleRepository {
	return &PostgresFlashSaleRepository{pool: pool, outbox: ob}
}

const campaignColumns = `id, listing_id, variant_id, sale_price, stock_cap, stock_sold, starts_at, ends_at`

func scanCampaign(row pgx.Row, c *FlashSaleCampaign) error {
	// starts_at / ends_at are nullable (NULL = open-ended, stored as the zero time).
	var starts, ends *time.Time
	if err := row.Scan(&c.ID, &c.ListingID, &c.VariantID, &c.SalePrice, &c.StockCap, &c.StockSold, &starts, &ends); err != nil {
		return err
	}
	c.StartsAt, c.EndsAt = time.Time{}, time.Time{}
	if starts != nil {
		c.StartsAt = *starts
	}
	if ends != nil {
		c.EndsAt = *ends
	}
	return nil
}

func (r *PostgresFlashSaleRepository) Create(ctx context.Context, c FlashSaleCampaign, event func(FlashSaleCampaign) (outbox.Row, error)) (FlashSaleCampaign, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	const q = `INSERT INTO flash_sale_campaigns (` + campaignColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, q,
			c.ID, c.ListingID, c.VariantID, c.SalePrice, c.StockCap, c.StockSold, nullableTime(c.StartsAt), nullableTime(c.EndsAt),
		); err != nil {
			return fmt.Errorf("insert campaign: %w", err)
		}
		row, err := event(c)
		if err != nil {
			return err
		}
		return r.outbox.EnqueueTx(ctx, tx, row)
	})
	if err != nil {
		return FlashSaleCampaign{}, err
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
