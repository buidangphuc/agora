package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

const (
	CampaignStatusScheduled = "scheduled"
	CampaignStatusActive    = "active"
	CampaignStatusEnded     = "ended"
	CampaignStatusCancelled = "cancelled"
)

const idempotencyTTL = 24 * time.Hour

var (
	ErrCampaignEnded        = errors.New("flash sale campaign ended")
	ErrCampaignNotScheduled = errors.New("flash sale campaign not scheduled")
	ErrCampaignCancelled    = errors.New("flash sale campaign cancelled")
	ErrIdempotencyMismatch  = errors.New("idempotency key reused with a different payload")
	ErrIdempotencyNotFound  = errors.New("idempotency record not found")
)

type IdempotentCampaign struct {
	Campaign    FlashSaleCampaign
	PayloadHash string
}

func (r *PostgresFlashSaleRepository) ListCampaigns(ctx context.Context, statuses []string, now time.Time, offset, limit int) ([]FlashSaleCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const q = `SELECT ` + campaignSelectColumns + ` FROM flash_sale_campaigns
		WHERE cardinality($1::text[]) = 0 OR (CASE
			WHEN cancelled_at IS NOT NULL THEN 'cancelled'
			WHEN starts_at IS NOT NULL AND starts_at > $2 THEN 'scheduled'
			WHEN ends_at IS NOT NULL AND ends_at <= $2 THEN 'ended'
			ELSE 'active' END) = ANY($1::text[])
		ORDER BY starts_at DESC NULLS LAST, id ASC
		LIMIT $3 OFFSET $4`
	if statuses == nil {
		statuses = []string{}
	}
	rows, err := r.pool.Query(ctx, q, statuses, now, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	out := []FlashSaleCampaign{}
	for rows.Next() {
		var c FlashSaleCampaign
		if err := scanCampaign(rows, &c); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func lockCampaignState(ctx context.Context, tx pgx.Tx, id string) (starts, ends, cancelled *time.Time, err error) {
	err = tx.QueryRow(ctx,
		`SELECT starts_at, ends_at, cancelled_at FROM flash_sale_campaigns WHERE id = $1 FOR UPDATE`, id,
	).Scan(&starts, &ends, &cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, ErrCampaignNotFound
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lock campaign %q: %w", id, err)
	}
	return starts, ends, cancelled, nil
}

func (r *PostgresFlashSaleRepository) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	var at time.Time
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		_, ends, cancelled, err := lockCampaignState(ctx, tx, id)
		if err != nil {
			return err
		}
		if cancelled != nil {
			at = *cancelled
			return nil
		}
		if ends != nil && !ends.After(now) {
			return ErrCampaignEnded
		}
		if _, err := tx.Exec(ctx, `UPDATE flash_sale_campaigns SET cancelled_at = $2, updated_at = $2 WHERE id = $1`, id, now); err != nil {
			return fmt.Errorf("cancel campaign %q: %w", id, err)
		}
		at = now
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return at, nil
}

func (r *PostgresFlashSaleRepository) UpdateIfScheduled(ctx context.Context, id string, c FlashSaleCampaign, now time.Time) (FlashSaleCampaign, error) {
	var out FlashSaleCampaign
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		starts, _, cancelled, err := lockCampaignState(ctx, tx, id)
		if err != nil {
			return err
		}
		if cancelled != nil || starts == nil || !starts.After(now) {
			return ErrCampaignNotScheduled
		}
		if _, err := tx.Exec(ctx,
			`UPDATE flash_sale_campaigns SET sale_price = $2, stock_cap = $3, starts_at = $4, ends_at = $5, updated_at = $6 WHERE id = $1`,
			id, c.SalePrice, c.StockCap, nullableTime(c.StartsAt), nullableTime(c.EndsAt), now,
		); err != nil {
			return fmt.Errorf("update campaign %q: %w", id, err)
		}
		return scanCampaign(tx.QueryRow(ctx, `SELECT `+campaignSelectColumns+` FROM flash_sale_campaigns WHERE id = $1`, id), &out)
	})
	if err != nil {
		return FlashSaleCampaign{}, err
	}
	return out, nil
}

func (r *PostgresFlashSaleRepository) LookupIdempotent(ctx context.Context, actorID, key string, now time.Time) (IdempotentCampaign, error) {
	var out IdempotentCampaign
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		found, err := lookupIdempotentTx(ctx, tx, actorID, key, now, &out)
		if err != nil {
			return err
		}
		if !found {
			return ErrIdempotencyNotFound
		}
		return nil
	})
	if err != nil {
		return IdempotentCampaign{}, err
	}
	return out, nil
}

func lookupIdempotentTx(ctx context.Context, tx pgx.Tx, actorID, key string, now time.Time, out *IdempotentCampaign) (bool, error) {
	var campaignID string
	err := tx.QueryRow(ctx,
		`SELECT payload_hash, campaign_id FROM campaign_idempotency WHERE actor_id = $1 AND key = $2 AND created_at > $3`,
		actorID, key, now.Add(-idempotencyTTL),
	).Scan(&out.PayloadHash, &campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup idempotency: %w", err)
	}
	if err := scanCampaign(tx.QueryRow(ctx, `SELECT `+campaignSelectColumns+` FROM flash_sale_campaigns WHERE id = $1`, campaignID), &out.Campaign); err != nil {
		return false, fmt.Errorf("load idempotent campaign %q: %w", campaignID, err)
	}
	return true, nil
}

func (r *PostgresFlashSaleRepository) CreateIdempotent(ctx context.Context, actorID, key, payloadHash string, c FlashSaleCampaign, now time.Time, event func(FlashSaleCampaign) (outbox.Row, error)) (FlashSaleCampaign, bool, error) {
	var out FlashSaleCampaign
	var created bool
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("%d:%s:%s", len(actorID), actorID, key)); err != nil {
			return fmt.Errorf("lock idempotency key: %w", err)
		}
		var hit IdempotentCampaign
		found, err := lookupIdempotentTx(ctx, tx, actorID, key, now, &hit)
		if err != nil {
			return err
		}
		if found {
			if hit.PayloadHash != payloadHash {
				return ErrIdempotencyMismatch
			}
			out = hit.Campaign
			return nil
		}
		if _, err := tx.Exec(ctx, `DELETE FROM campaign_idempotency WHERE actor_id = $1 AND key = $2`, actorID, key); err != nil {
			return fmt.Errorf("drop expired idempotency: %w", err)
		}
		if c.ID == "" {
			c.ID = NewID()
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO flash_sale_campaigns (`+campaignColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			c.ID, c.ListingID, c.VariantID, c.SalePrice, c.StockCap, c.StockSold, nullableTime(c.StartsAt), nullableTime(c.EndsAt),
		); err != nil {
			return fmt.Errorf("insert campaign: %w", err)
		}
		row, err := event(c)
		if err != nil {
			return err
		}
		if err := r.outbox.EnqueueTx(ctx, tx, row); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO campaign_idempotency (actor_id, key, payload_hash, campaign_id, created_at) VALUES ($1, $2, $3, $4, $5)`,
			actorID, key, payloadHash, c.ID, now,
		); err != nil {
			return fmt.Errorf("insert idempotency: %w", err)
		}
		out, created = c, true
		return nil
	})
	if err != nil {
		return FlashSaleCampaign{}, false, err
	}
	return out, created, nil
}

func (r *PostgresFlashSaleRepository) PurgeIdempotency(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM campaign_idempotency WHERE created_at <= $1`, now.Add(-idempotencyTTL))
	if err != nil {
		return 0, fmt.Errorf("purge idempotency: %w", err)
	}
	return tag.RowsAffected(), nil
}
