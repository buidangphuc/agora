package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAdCampaignNotFound is returned when an ad-campaign lookup misses.
var ErrAdCampaignNotFound = errors.New("ad campaign not found")

// AdCampaign status enum values (mirror platform.promotion.v1.AdCampaignStatus
// without importing the wire enum into the persistence layer).
const (
	AdCampaignStatusActive int32 = 1
	AdCampaignStatusPaused int32 = 2
	AdCampaignStatusEnded  int32 = 3
)

// AdCampaign is the persistence-layer view of a sponsored-listing campaign. Field
// semantics mirror platform.promotion.v1.AdCampaign; the mapping to/from the wire
// type lives in the service layer. Budget/bid are pure bookkeeping (minor units) —
// this service never moves money (AGENTS.md §7).
type AdCampaign struct {
	ID        string
	SellerID  string
	ListingID string
	Budget    int64
	Bid       int64
	Status    int32
	CreatedAt time.Time
}

// AdCampaignRepository is the persistence seam for sponsored ad campaigns.
type AdCampaignRepository interface {
	// Create inserts a new campaign. A blank ID is filled with a generated one; a
	// zero status defaults to active.
	Create(ctx context.Context, c AdCampaign) (AdCampaign, error)
	// GetByID looks a campaign up by surrogate id (ErrAdCampaignNotFound on miss).
	GetByID(ctx context.Context, id string) (AdCampaign, error)
	// ListBySeller returns a seller's campaigns, newest first, bounded by
	// limit/offset.
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]AdCampaign, error)
	// ListActive returns the active campaigns ordered by bid descending (best bid
	// first), bounded by limit. Ties break on newest-first for a stable order.
	ListActive(ctx context.Context, limit int) ([]AdCampaign, error)
	// Ping verifies the backing store is reachable.
	Ping(ctx context.Context) error
}

// ── Postgres implementation ──

// PostgresAdCampaignRepository persists sponsored campaigns in promotion_db.
type PostgresAdCampaignRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAdCampaignRepository(pool *pgxpool.Pool) *PostgresAdCampaignRepository {
	return &PostgresAdCampaignRepository{pool: pool}
}

const adCampaignColumns = `id, seller_id, listing_id, budget, bid, status, created_at`

func scanAdCampaign(row pgx.Row, c *AdCampaign) error {
	return row.Scan(&c.ID, &c.SellerID, &c.ListingID, &c.Budget, &c.Bid, &c.Status, &c.CreatedAt)
}

func (r *PostgresAdCampaignRepository) Create(ctx context.Context, c AdCampaign) (AdCampaign, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.Status == 0 {
		c.Status = AdCampaignStatusActive
	}
	const q = `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + adCampaignColumns
	var out AdCampaign
	if err := scanAdCampaign(
		r.pool.QueryRow(ctx, q, c.ID, c.SellerID, c.ListingID, c.Budget, c.Bid, c.Status), &out,
	); err != nil {
		return AdCampaign{}, fmt.Errorf("insert ad campaign: %w", err)
	}
	return out, nil
}

func (r *PostgresAdCampaignRepository) GetByID(ctx context.Context, id string) (AdCampaign, error) {
	const q = `SELECT ` + adCampaignColumns + ` FROM ad_campaigns WHERE id = $1`
	var c AdCampaign
	if err := scanAdCampaign(r.pool.QueryRow(ctx, q, id), &c); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdCampaign{}, ErrAdCampaignNotFound
		}
		return AdCampaign{}, fmt.Errorf("get ad campaign by id %q: %w", id, err)
	}
	return c, nil
}

func (r *PostgresAdCampaignRepository) ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]AdCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT ` + adCampaignColumns + ` FROM ad_campaigns WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, sellerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list ad campaigns: %w", err)
	}
	defer rows.Close()
	var out []AdCampaign
	for rows.Next() {
		var c AdCampaign
		if err := scanAdCampaign(rows, &c); err != nil {
			return nil, fmt.Errorf("scan ad campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresAdCampaignRepository) ListActive(ctx context.Context, limit int) ([]AdCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT ` + adCampaignColumns + ` FROM ad_campaigns WHERE status = $1 ORDER BY bid DESC, created_at DESC LIMIT $2`
	rows, err := r.pool.Query(ctx, q, AdCampaignStatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("list active ad campaigns: %w", err)
	}
	defer rows.Close()
	var out []AdCampaign
	for rows.Next() {
		var c AdCampaign
		if err := scanAdCampaign(rows, &c); err != nil {
			return nil, fmt.Errorf("scan ad campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresAdCampaignRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
