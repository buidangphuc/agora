package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ProcessedEvents struct {
	q Querier
}

func NewProcessedEvents(q Querier) *ProcessedEvents {
	return &ProcessedEvents{q: q}
}

func (r *ProcessedEvents) IsProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	var one int
	err := r.q.QueryRow(ctx,
		`SELECT 1 FROM processed_events WHERE consumer = $1 AND event_id = $2`, consumer, eventID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup processed event: %w", err)
	}
	return true, nil
}

func (r *ProcessedEvents) MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	tag, err := r.q.Exec(ctx,
		`INSERT INTO processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("mark processed event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

type ListingPrices struct {
	q Querier
}

func NewListingPrices(q Querier) *ListingPrices {
	return &ListingPrices{q: q}
}

func (s *ListingPrices) Get(ctx context.Context, listingID string) (int64, bool, error) {
	var price *int64
	err := s.q.QueryRow(ctx, `SELECT price FROM listing_last_seen WHERE listing_id = $1`, listingID).Scan(&price)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read last-seen price: %w", err)
	}
	if price == nil {
		return 0, false, nil
	}
	return *price, true, nil
}

func (s *ListingPrices) Set(ctx context.Context, listingID string, price int64) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO listing_last_seen (listing_id, price) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET price = EXCLUDED.price, updated_at = NOW()`, listingID, price)
	if err != nil {
		return fmt.Errorf("record last-seen price: %w", err)
	}
	return nil
}

type ListingStocks struct {
	q Querier
}

func NewListingStocks(q Querier) *ListingStocks {
	return &ListingStocks{q: q}
}

func (s *ListingStocks) Get(ctx context.Context, listingID string) (int32, bool, error) {
	var stock *int32
	err := s.q.QueryRow(ctx, `SELECT stock FROM listing_last_seen WHERE listing_id = $1`, listingID).Scan(&stock)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read last-seen stock: %w", err)
	}
	if stock == nil {
		return 0, false, nil
	}
	return *stock, true, nil
}

func (s *ListingStocks) Set(ctx context.Context, listingID string, stock int32) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO listing_last_seen (listing_id, stock) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET stock = EXCLUDED.stock, updated_at = NOW()`, listingID, stock)
	if err != nil {
		return fmt.Errorf("record last-seen stock: %w", err)
	}
	return nil
}

type LowStock struct {
	q Querier
}

func NewLowStock(q Querier) *LowStock {
	return &LowStock{q: q}
}

func (s *LowStock) Get(ctx context.Context, listingID string) (sellerID, title string, low, known bool, err error) {
	err = s.q.QueryRow(ctx,
		`SELECT seller_id, title, low FROM listing_low_stock_state WHERE listing_id = $1`, listingID).
		Scan(&sellerID, &title, &low)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, false, nil
	}
	if err != nil {
		return "", "", false, false, fmt.Errorf("read low-stock state: %w", err)
	}
	return sellerID, title, low, true, nil
}

func (s *LowStock) Upsert(ctx context.Context, listingID, sellerID, title string, low bool) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO listing_low_stock_state (listing_id, seller_id, title, low) VALUES ($1, $2, $3, $4)
		ON CONFLICT (listing_id) DO UPDATE
		SET seller_id = EXCLUDED.seller_id, title = EXCLUDED.title, low = EXCLUDED.low, updated_at = NOW()`,
		listingID, sellerID, title, low)
	if err != nil {
		return fmt.Errorf("record low-stock state: %w", err)
	}
	return nil
}

func (s *LowStock) SetLow(ctx context.Context, listingID string, low bool) error {
	_, err := s.q.Exec(ctx,
		`UPDATE listing_low_stock_state SET low = $2, updated_at = NOW() WHERE listing_id = $1`, listingID, low)
	if err != nil {
		return fmt.Errorf("set low-stock flag: %w", err)
	}
	return nil
}

func (s *LowStock) Delete(ctx context.Context, listingID string) error {
	_, err := s.q.Exec(ctx, `DELETE FROM listing_low_stock_state WHERE listing_id = $1`, listingID)
	if err != nil {
		return fmt.Errorf("delete low-stock state: %w", err)
	}
	return nil
}
