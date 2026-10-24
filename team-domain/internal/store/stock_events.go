package store

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

// StockSnapshot is what a ListingStockChanged event announces: the listing's
// stock AFTER a change and, when variants changed, those variants with their
// stock after the change. Unchanged variants are not included.
type StockSnapshot struct {
	ListingID string
	Stock     int32
	Variants  []Variant
}

// StockEventBuilder builds the outbox row announcing a stock change. The
// repository calls it INSIDE the reserve / release / sweep transaction, so the
// row commits (or rolls back) with the stock change; a builder error rolls the
// stock change back. Idempotent no-ops (repeated reserve, repeated or unknown
// release, commit) never call it.
type StockEventBuilder func(ctx context.Context, snap StockSnapshot) (OutboxRow, error)

// stockChanges collects, within one transaction, the listings whose stock really
// changed and, per listing, the variants that changed ("" = base stock).
type stockChanges map[string]map[string]bool

func (c stockChanges) add(listingID, variantID string) {
	if c[listingID] == nil {
		c[listingID] = map[string]bool{}
	}
	if variantID != "" {
		c[listingID][variantID] = true
	}
}

// listings returns the changed listing ids in a stable order.
func (c stockChanges) listings() []string {
	out := make([]string, 0, len(c))
	for id := range c {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (c stockChanges) variants(listingID string) []string {
	out := make([]string, 0, len(c[listingID]))
	for id := range c[listingID] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// WithStockEvents makes every real stock change (reserve, release, sweep)
// enqueue one ListingStockChanged row per affected listing on outbox, inside the
// stock-change transaction. Without it (DB-only tests, outbox disabled) no event
// is written. Returns the repo for chaining.
func (r *PostgresListingRepository) WithStockEvents(outbox *outbox.Store, b StockEventBuilder) *PostgresListingRepository {
	r.outbox, r.stockEvent = outbox, b
	return r
}

// emitStockChanges enqueues one ListingStockChanged row per changed listing over
// tx, reading the post-change stock inside the same transaction. A listing
// deleted meanwhile announces nothing. No-op when events are not configured.
func (r *PostgresListingRepository) emitStockChanges(ctx context.Context, tx pgx.Tx, changes stockChanges) error {
	if r.outbox == nil || r.stockEvent == nil {
		return nil
	}
	for _, listingID := range changes.listings() {
		snap := StockSnapshot{ListingID: listingID}
		if err := tx.QueryRow(ctx, `SELECT stock FROM listings WHERE id = $1`, listingID).Scan(&snap.Stock); err != nil {
			if isNoRows(err) {
				continue
			}
			return fmt.Errorf("read stock of %q: %w", listingID, err)
		}
		if ids := changes.variants(listingID); len(ids) > 0 {
			rows, err := tx.Query(ctx, `SELECT id, listing_id, name, sku, price, stock, image_url
				FROM listing_variants WHERE listing_id = $1 AND id = ANY($2) ORDER BY id`, listingID, ids)
			if err != nil {
				return fmt.Errorf("read variant stock of %q: %w", listingID, err)
			}
			for rows.Next() {
				var v Variant
				if err := rows.Scan(&v.ID, &v.ListingID, &v.Name, &v.SKU, &v.Price, &v.Stock, &v.ImageURL); err != nil {
					rows.Close()
					return fmt.Errorf("scan variant: %w", err)
				}
				snap.Variants = append(snap.Variants, v)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterate variants: %w", err)
			}
		}
		row, err := r.stockEvent(ctx, snap)
		if err != nil {
			return fmt.Errorf("build stock event: %w", err)
		}
		if err := r.outbox.EnqueueTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}
