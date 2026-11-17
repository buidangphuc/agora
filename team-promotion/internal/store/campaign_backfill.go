package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
)

func (r *PostgresFlashSaleRepository) ListUnassignedCampaigns(ctx context.Context, afterID string, limit int) ([]FlashSaleCampaign, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+campaignSelectColumns+` FROM flash_sale_campaigns WHERE seller_id = '' AND id > $1 ORDER BY id LIMIT $2`,
		afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list unassigned campaigns: %w", err)
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

func (r *PostgresFlashSaleRepository) AssignCampaignSellers(ctx context.Context, sellers map[string]string) (int, error) {
	updated := 0
	err := postgres.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		updated = 0
		for id, seller := range sellers {
			if seller == "" {
				continue
			}
			tag, err := tx.Exec(ctx, `UPDATE flash_sale_campaigns SET seller_id = $2 WHERE id = $1 AND seller_id = ''`, id, seller)
			if err != nil {
				return fmt.Errorf("assign campaign seller %q: %w", id, err)
			}
			updated += int(tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}
