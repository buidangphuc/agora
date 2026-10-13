package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
)

// PostgresProcessedEventRepo is the durable dedupe ledger (processed_events,
// migration 004). It satisfies events.Deduper over the SDK's consume.Dedupe: the
// zero Dedupe uses that default table, whose columns are the SDK ledger's
// (consumer, event_id, processed_at). The lookup (Processed) and the mark (Seen)
// are separate because this service records an event after its effect, not before.
type PostgresProcessedEventRepo struct {
	pool   *pgxpool.Pool
	dedupe consume.Dedupe
}

// NewPostgresProcessedEventRepo returns the ledger over pool.
func NewPostgresProcessedEventRepo(pool *pgxpool.Pool) *PostgresProcessedEventRepo {
	return &PostgresProcessedEventRepo{pool: pool}
}

// IsProcessed reports whether eventID was already applied for consumer.
func (r *PostgresProcessedEventRepo) IsProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	processed, err := r.dedupe.Processed(ctx, r.pool, consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("lookup processed event: %w", err)
	}
	return processed, nil
}

// MarkProcessed records eventID; it reports whether the row was newly inserted.
func (r *PostgresProcessedEventRepo) MarkProcessed(ctx context.Context, consumer, eventID string) (bool, error) {
	seen, err := r.dedupe.Seen(ctx, r.pool, consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("mark processed event: %w", err)
	}
	return !seen, nil
}

// PostgresListingPriceStore keeps the last-seen price per listing in
// listing_last_seen. It satisfies consumer.PriceStateStore.
type PostgresListingPriceStore struct {
	pool *pgxpool.Pool
}

// NewPostgresListingPriceStore returns the price store over pool.
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

// NewPostgresListingStockStore returns the stock store over pool.
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
