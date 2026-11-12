package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

type ListingSalesChange struct {
	ListingID     string
	SoldCount     int64
	ChangedAt     time.Time
	SourceEventID string
}

type ListingSalesOutboxBuilder func(change ListingSalesChange) (outbox.Row, error)

func WithListingSalesOutbox(b ListingSalesOutboxBuilder) OrderRepoOption {
	return func(c *orderRepoConfig) { c.listingSalesOutbox = b }
}

func WithReturnListingSalesOutbox(b ListingSalesOutboxBuilder) ReturnRepoOption {
	return func(c *returnRepoConfig) { c.listingSalesOutbox = b }
}

type salesDelta struct {
	listingID string
	quantity  int64
}

func countOrderSales(ctx context.Context, tx pgx.Tx, ob *outbox.Store, build ListingSalesOutboxBuilder, orderID string) error {
	return countOrderSalesAs(ctx, tx, ob, build, orderID, "paid")
}

func countOrderSalesAs(ctx context.Context, tx pgx.Tx, ob *outbox.Store, build ListingSalesOutboxBuilder, orderID, stage string) error {
	rows, err := tx.Query(ctx, `INSERT INTO listing_sales_orders (order_id, listing_id, quantity, state)
		SELECT order_id, listing_id, SUM(quantity), 'counted' FROM order_items WHERE order_id = $1 GROUP BY order_id, listing_id
		ON CONFLICT (order_id, listing_id) DO NOTHING
		RETURNING listing_id, quantity`, orderID)
	if err != nil {
		return fmt.Errorf("count order %q in listing sales: %w", orderID, err)
	}
	deltas, err := scanSalesDeltas(rows)
	if err != nil {
		return err
	}
	return applySalesDeltas(ctx, tx, ob, build, deltas, 1, "order:"+orderID+":"+stage)
}

func reverseOrderSales(ctx context.Context, tx pgx.Tx, ob *outbox.Store, build ListingSalesOutboxBuilder, orderID string) error {
	rows, err := tx.Query(ctx, `UPDATE listing_sales_orders SET state = 'reversed'
		WHERE order_id = $1 AND state = 'counted'
		RETURNING listing_id, quantity`, orderID)
	if err != nil {
		return fmt.Errorf("reverse order %q in listing sales: %w", orderID, err)
	}
	deltas, err := scanSalesDeltas(rows)
	if err != nil {
		return err
	}
	return applySalesDeltas(ctx, tx, ob, build, deltas, -1, "order:"+orderID+":reversed")
}

func scanSalesDeltas(rows pgx.Rows) ([]salesDelta, error) {
	defer rows.Close()
	var out []salesDelta
	for rows.Next() {
		var d salesDelta
		if err := rows.Scan(&d.listingID, &d.quantity); err != nil {
			return nil, fmt.Errorf("scan listing sales ledger: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read listing sales ledger: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].listingID < out[j].listingID })
	return out, nil
}

func applySalesDeltas(ctx context.Context, tx pgx.Tx, ob *outbox.Store, build ListingSalesOutboxBuilder, deltas []salesDelta, sign int64, source string) error {
	if len(deltas) == 0 {
		return nil
	}
	ids := make([]string, len(deltas))
	qty := make(map[string]int64, len(deltas))
	for i, d := range deltas {
		ids[i] = d.listingID
		qty[d.listingID] = d.quantity
	}
	if sign > 0 {
		return addSalesDeltas(ctx, tx, ob, build, deltas, source)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO listing_sales (listing_id, sold_count)
		SELECT unnest($1::text[]), 0 ON CONFLICT (listing_id) DO NOTHING`, ids); err != nil {
		return fmt.Errorf("init listing sales: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT listing_id, sold_count, now() FROM listing_sales
		WHERE listing_id = ANY($1::text[]) ORDER BY listing_id FOR UPDATE`, ids)
	if err != nil {
		return fmt.Errorf("lock listing sales: %w", err)
	}
	var changedAt time.Time
	var changed []ListingSalesChange
	var changedIDs []string
	var changedNext []int64
	for rows.Next() {
		var id string
		var old int64
		if err := rows.Scan(&id, &old, &changedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan listing sales: %w", err)
		}
		next := max(old+sign*qty[id], 0)
		if next == old {
			continue
		}
		changedIDs = append(changedIDs, id)
		changedNext = append(changedNext, next)
		changed = append(changed, ListingSalesChange{ListingID: id, SoldCount: next, SourceEventID: source})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("lock listing sales: %w", err)
	}
	if len(changed) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE listing_sales s SET sold_count = v.next
		FROM unnest($1::text[], $2::bigint[]) AS v(listing_id, next) WHERE s.listing_id = v.listing_id`, changedIDs, changedNext); err != nil {
		return fmt.Errorf("update listing sales: %w", err)
	}
	for _, c := range changed {
		c.ChangedAt = changedAt
		row, err := build(c)
		if err != nil {
			return fmt.Errorf("build listing sales outbox row: %w", err)
		}
		if err := ob.EnqueueTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

func addSalesDeltas(ctx context.Context, tx pgx.Tx, ob *outbox.Store, build ListingSalesOutboxBuilder, deltas []salesDelta, source string) error {
	ids := make([]string, len(deltas))
	qtys := make([]int64, len(deltas))
	for i, d := range deltas {
		ids[i] = d.listingID
		qtys[i] = d.quantity
	}
	rows, err := tx.Query(ctx, `INSERT INTO listing_sales AS s (listing_id, sold_count)
		SELECT v.listing_id, v.qty FROM unnest($1::text[], $2::bigint[]) AS v(listing_id, qty) ORDER BY v.listing_id
		ON CONFLICT (listing_id) DO UPDATE SET sold_count = s.sold_count + EXCLUDED.sold_count
		RETURNING s.listing_id, s.sold_count, now()`, ids, qtys)
	if err != nil {
		return fmt.Errorf("add listing sales: %w", err)
	}
	var changes []ListingSalesChange
	for rows.Next() {
		var c ListingSalesChange
		if err := rows.Scan(&c.ListingID, &c.SoldCount, &c.ChangedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan listing sales: %w", err)
		}
		c.SourceEventID = source
		changes = append(changes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("add listing sales: %w", err)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ListingID < changes[j].ListingID })
	for _, c := range changes {
		row, err := build(c)
		if err != nil {
			return fmt.Errorf("build listing sales outbox row: %w", err)
		}
		if err := ob.EnqueueTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}
