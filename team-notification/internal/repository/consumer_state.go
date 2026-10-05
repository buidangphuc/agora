package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresProcessedEventRepo is the durable dedupe ledger (processed_events). It
// satisfies consumer.Deduper.
type PostgresProcessedEventRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresProcessedEventRepo(pool *pgxpool.Pool) *PostgresProcessedEventRepo {
	return &PostgresProcessedEventRepo{pool: pool}
}

// IsProcessed reports whether eventID was already applied for consumer.
func (r *PostgresProcessedEventRepo) IsProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM processed_events WHERE consumer = $1 AND event_id = $2`, consumer, eventID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup processed event: %w", err)
	}
	return true, nil
}

// MarkProcessed records eventID; it reports whether the row was newly inserted.
func (r *PostgresProcessedEventRepo) MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("mark processed event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// PostgresListingPriceStore keeps the last-seen price per listing in
// listing_last_seen. It satisfies consumer.PriceStateStore.
type PostgresListingPriceStore struct {
	pool *pgxpool.Pool
}

func NewPostgresListingPriceStore(pool *pgxpool.Pool) *PostgresListingPriceStore {
	return &PostgresListingPriceStore{pool: pool}
}

// Get returns the last-seen price; known is false when none was recorded.
func (s *PostgresListingPriceStore) Get(ctx context.Context, listingID string) (int64, bool, error) {
	var price *int64
	err := s.pool.QueryRow(ctx, `SELECT price FROM listing_last_seen WHERE listing_id = $1`, listingID).Scan(&price)
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

// Set upserts the last-seen price, leaving the stock column untouched.
func (s *PostgresListingPriceStore) Set(ctx context.Context, listingID string, price int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO listing_last_seen (listing_id, price) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET price = EXCLUDED.price, updated_at = NOW()`, listingID, price)
	if err != nil {
		return fmt.Errorf("record last-seen price: %w", err)
	}
	return nil
}

// PostgresListingStockStore keeps the last-seen stock per listing in
// listing_last_seen. It satisfies consumer.StockStateStore.
type PostgresListingStockStore struct {
	pool *pgxpool.Pool
}

func NewPostgresListingStockStore(pool *pgxpool.Pool) *PostgresListingStockStore {
	return &PostgresListingStockStore{pool: pool}
}

// Get returns the last-seen stock; known is false when none was recorded.
func (s *PostgresListingStockStore) Get(ctx context.Context, listingID string) (int32, bool, error) {
	var stock *int32
	err := s.pool.QueryRow(ctx, `SELECT stock FROM listing_last_seen WHERE listing_id = $1`, listingID).Scan(&stock)
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

// Set upserts the last-seen stock, leaving the price column untouched.
func (s *PostgresListingStockStore) Set(ctx context.Context, listingID string, stock int32) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO listing_last_seen (listing_id, stock) VALUES ($1, $2)
		ON CONFLICT (listing_id) DO UPDATE SET stock = EXCLUDED.stock, updated_at = NOW()`, listingID, stock)
	if err != nil {
		return fmt.Errorf("record last-seen stock: %w", err)
	}
	return nil
}
