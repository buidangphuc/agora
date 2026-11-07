package store

import (
	"context"
	"errors"
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
	return applySalesDeltas(ctx, tx, ob, build, deltas, 1, "order:"+orderID+":paid")
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
	var changedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&changedAt); err != nil {
		return fmt.Errorf("read transaction time: %w", err)
	}
	for _, d := range deltas {
		if _, err := tx.Exec(ctx, `INSERT INTO listing_sales (listing_id, sold_count) VALUES ($1, 0) ON CONFLICT (listing_id) DO NOTHING`, d.listingID); err != nil {
			return fmt.Errorf("init listing sales %q: %w", d.listingID, err)
		}
		var old int64
		if err := tx.QueryRow(ctx, `SELECT sold_count FROM listing_sales WHERE listing_id = $1 FOR UPDATE`, d.listingID).Scan(&old); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock listing sales %q: %w", d.listingID, err)
		}
		next := max(old+sign*d.quantity, 0)
		if next == old {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE listing_sales SET sold_count = $2 WHERE listing_id = $1`, d.listingID, next); err != nil {
			return fmt.Errorf("update listing sales %q: %w", d.listingID, err)
		}
		row, err := build(ListingSalesChange{ListingID: d.listingID, SoldCount: next, ChangedAt: changedAt, SourceEventID: source})
		if err != nil {
			return fmt.Errorf("build listing sales outbox row: %w", err)
		}
		if err := ob.EnqueueTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}
