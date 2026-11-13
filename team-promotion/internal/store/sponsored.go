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

	AdCampaignStatusPausedBudget int32 = 4
)

const ImpressionTTL = 30 * time.Minute

var ErrImpressionNotFound = errors.New("impression token not found")

type ClickResult struct {
	ListingID string
	Charged   bool
}

type ImpressionIssue struct {
	Token      string
	CampaignID string
	ListingID  string
}

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

	Spent       int64
	Impressions int64
	Clicks      int64
	CategoryID  string
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
	ListServable(ctx context.Context, category string, limit int) ([]AdCampaign, error)
	IssueImpressions(ctx context.Context, issues []ImpressionIssue) error
	// RecordClick consumes token at instant now and charges the campaign's bid in one
	// transaction. principalID is the forwarded caller ("" when anonymous); a principal
	// equal to the campaign's seller is never charged and leaves the token unconsumed.
	RecordClick(ctx context.Context, token, principalID string, now time.Time) (ClickResult, error)
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

const adCampaignColumns = `id, seller_id, listing_id, budget, bid, status, created_at, spent, impressions, clicks, category_id`

func scanAdCampaign(row pgx.Row, c *AdCampaign) error {
	return row.Scan(&c.ID, &c.SellerID, &c.ListingID, &c.Budget, &c.Bid, &c.Status, &c.CreatedAt, &c.Spent, &c.Impressions, &c.Clicks, &c.CategoryID)
}

func (r *PostgresAdCampaignRepository) Create(ctx context.Context, c AdCampaign) (AdCampaign, error) {
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.Status == 0 {
		c.Status = AdCampaignStatusActive
	}
	const q = `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status, category_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + adCampaignColumns
	var out AdCampaign
	if err := scanAdCampaign(
		r.pool.QueryRow(ctx, q, c.ID, c.SellerID, c.ListingID, c.Budget, c.Bid, c.Status, c.CategoryID), &out,
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

func (r *PostgresAdCampaignRepository) ListServable(ctx context.Context, category string, limit int) ([]AdCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT ` + adCampaignColumns + ` FROM ad_campaigns
		WHERE status = $1 AND budget - spent >= bid AND ($2 = '' OR category_id = $2)
		ORDER BY bid DESC, created_at ASC, id ASC LIMIT $3`
	rows, err := r.pool.Query(ctx, q, AdCampaignStatusActive, category, limit)
	if err != nil {
		return nil, fmt.Errorf("list servable ad campaigns: %w", err)
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

func (r *PostgresAdCampaignRepository) IssueImpressions(ctx context.Context, issues []ImpressionIssue) error {
	if len(issues) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin impressions: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, i := range issues {
		if _, err := tx.Exec(ctx, `INSERT INTO ad_impressions (token, campaign_id, listing_id, expires_at)
			VALUES ($1, $2, $3, now() + $4::interval)`, i.Token, i.CampaignID, i.ListingID, ImpressionTTL.String()); err != nil {
			return fmt.Errorf("insert ad_impressions: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE ad_campaigns SET impressions = impressions + 1 WHERE id = $1`, i.CampaignID); err != nil {
			return fmt.Errorf("count impression: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresAdCampaignRepository) RecordClick(ctx context.Context, token, principalID string, now time.Time) (ClickResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ClickResult{}, fmt.Errorf("begin click: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var (
		campaignID, listingID string
		expiresAt             time.Time
		clickedAt             *time.Time
	)
	err = tx.QueryRow(ctx, `SELECT campaign_id, listing_id, expires_at, clicked_at FROM ad_impressions
		WHERE token = $1 FOR UPDATE`, token).Scan(&campaignID, &listingID, &expiresAt, &clickedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClickResult{}, ErrImpressionNotFound
	}
	if err != nil {
		return ClickResult{}, fmt.Errorf("read impression: %w", err)
	}
	res := ClickResult{ListingID: listingID}
	if clickedAt != nil || now.After(expiresAt) {
		return res, nil
	}
	var sellerID string
	var budget, spent, bid int64
	var status int32
	if err := tx.QueryRow(ctx, `SELECT seller_id, budget, spent, bid, status FROM ad_campaigns
		WHERE id = $1 FOR UPDATE`, campaignID).Scan(&sellerID, &budget, &spent, &bid, &status); err != nil {
		return ClickResult{}, fmt.Errorf("lock campaign: %w", err)
	}
	if principalID != "" && principalID == sellerID {
		return res, nil
	}
	if status != AdCampaignStatusActive || budget-spent < bid {
		return res, nil
	}
	newStatus := status
	if budget-spent-bid < bid {
		newStatus = AdCampaignStatusPausedBudget
	}
	if _, err := tx.Exec(ctx, `UPDATE ad_campaigns SET spent = spent + $2, clicks = clicks + 1, status = $3 WHERE id = $1`,
		campaignID, bid, newStatus); err != nil {
		return ClickResult{}, fmt.Errorf("charge click: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ad_impressions SET clicked_at = $2 WHERE token = $1`, token, now); err != nil {
		return ClickResult{}, fmt.Errorf("consume impression: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ClickResult{}, fmt.Errorf("commit click: %w", err)
	}
	res.Charged = true
	return res, nil
}
