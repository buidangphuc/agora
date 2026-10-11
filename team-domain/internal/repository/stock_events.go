package repository

import (
	"context"
	"fmt"
	"sort"
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
func (r *PostgresListingRepository) WithStockEvents(outbox *OutboxStore, b StockEventBuilder) *PostgresListingRepository {
	r.outbox, r.stockEvent = outbox, b
	return r
}

// emitStockChanges enqueues one ListingStockChanged row per changed listing over
// tx, reading the post-change stock inside the same transaction. A listing
// deleted meanwhile announces nothing. No-op when events are not configured.
func (r *PostgresListingRepository) emitStockChanges(ctx context.Context, tx DBTX, changes stockChanges) error {
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

// WithStockEvents enables stock-change events on the in-memory repo and returns
// it for chaining.
func (r *InMemoryListingRepository) WithStockEvents(b StockEventBuilder) *InMemoryListingRepository {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stockEvent = b
	return r
}

// StockEventRows returns a copy of the stock-change outbox rows recorded so far
// (the in-memory analogue of the pending rows the Postgres repo writes).
func (r *InMemoryListingRepository) StockEventRows() []OutboxRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]OutboxRow(nil), r.stockRows...)
}

// txLocked runs mutate as an all-or-nothing unit (the caller holds r.mu): the
// listings and reservations are snapshotted, and a mutate or event-builder error
// restores them and records no event, mirroring a rolled-back transaction.
// mutate records every real stock change in changes; one event row per changed
// listing is built after mutate succeeds.
func (r *InMemoryListingRepository) txLocked(ctx context.Context, mutate func(changes stockChanges) error) error {
	savedListings := make(map[string]Listing, len(r.byID))
	for id, l := range r.byID {
		l.Variants = append([]Variant(nil), l.Variants...)
		savedListings[id] = l
	}
	savedRes := make(map[string]memReservation, len(r.reservations))
	for id, v := range r.reservations {
		savedRes[id] = v
	}
	rollback := func() { r.byID, r.reservations = savedListings, savedRes }

	changes := stockChanges{}
	if err := mutate(changes); err != nil {
		rollback()
		return err
	}
	if r.stockEvent == nil {
		return nil
	}
	var rows []OutboxRow
	for _, listingID := range changes.listings() {
		l, ok := r.byID[listingID]
		if !ok {
			continue
		}
		snap := StockSnapshot{ListingID: listingID, Stock: l.Stock}
		changed := changes[listingID]
		for _, v := range l.Variants {
			if changed[v.ID] {
				snap.Variants = append(snap.Variants, v)
			}
		}
		row, err := r.stockEvent(ctx, snap)
		if err != nil {
			rollback()
			return err
		}
		rows = append(rows, row)
	}
	r.stockRows = append(r.stockRows, rows...)
	return nil
}
