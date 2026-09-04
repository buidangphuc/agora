package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the production store (engagement_db, Rule 3).
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) AddFavorite(ctx context.Context, userID, listingID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO favorites (user_id, listing_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, listingID)
	if err != nil {
		return false, fmt.Errorf("add favorite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil // already favorited
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO listing_stats (listing_id, favorite_count) VALUES ($1, 1)
		 ON CONFLICT (listing_id) DO UPDATE SET favorite_count = listing_stats.favorite_count + 1`,
		listingID); err != nil {
		return true, fmt.Errorf("bump favorite_count: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) RemoveFavorite(ctx context.Context, userID, listingID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1 AND listing_id = $2`, userID, listingID)
	if err != nil {
		return false, fmt.Errorf("remove favorite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := r.pool.Exec(ctx,
		`UPDATE listing_stats SET favorite_count = GREATEST(favorite_count - 1, 0) WHERE listing_id = $1`,
		listingID); err != nil {
		return true, fmt.Errorf("drop favorite_count: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) IsFavorite(ctx context.Context, userID, listingID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM favorites WHERE user_id = $1 AND listing_id = $2)`,
		userID, listingID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is favorite: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) ListFavorites(ctx context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	limit := clampPageSize(pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT listing_id FROM favorites
		 WHERE user_id = $1 AND ($2 = '' OR listing_id > $2)
		 ORDER BY listing_id LIMIT $3`,
		userID, cursor, limit+1)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", 0, fmt.Errorf("scan favorite: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM favorites WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count favorites: %w", err)
	}
	return ids, next, total, nil
}

func (r *PostgresRepository) RecordView(ctx context.Context, userID, listingID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO listing_stats (listing_id, view_count) VALUES ($1, 1)
		 ON CONFLICT (listing_id) DO UPDATE SET view_count = listing_stats.view_count + 1
		 RETURNING view_count`,
		listingID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("record view: %w", err)
	}
	// Authenticated views also land in the caller's recently-viewed history:
	// upsert dedups on (user_id, listing_id) and bumps viewed_at to now().
	// Anonymous callers (empty userID) only touch the aggregate counter.
	if userID != "" {
		if _, err := r.pool.Exec(ctx,
			`INSERT INTO view_history (user_id, listing_id, viewed_at) VALUES ($1, $2, now())
			 ON CONFLICT (user_id, listing_id) DO UPDATE SET viewed_at = now()`,
			userID, listingID); err != nil {
			return count, fmt.Errorf("record view history: %w", err)
		}
	}
	return count, nil
}

func (r *PostgresRepository) GetRecentlyViewed(ctx context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	if userID == "" {
		return []string{}, "", 0, nil
	}
	limit := clampPageSize(pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT listing_id, viewed_at FROM view_history
		 WHERE user_id = $1 AND ($2 = '' OR viewed_at < $2::timestamptz)
		 ORDER BY viewed_at DESC LIMIT $3`,
		userID, cursor, limit+1)
	if err != nil {
		return nil, "", 0, fmt.Errorf("recently viewed: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	viewedAts := make([]time.Time, 0, limit)
	for rows.Next() {
		var (
			id       string
			viewedAt time.Time
		)
		if err := rows.Scan(&id, &viewedAt); err != nil {
			return nil, "", 0, fmt.Errorf("scan recently viewed: %w", err)
		}
		ids = append(ids, id)
		viewedAts = append(viewedAts, viewedAt)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		next = viewedAts[limit-1].UTC().Format(time.RFC3339Nano)
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM view_history WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count recently viewed: %w", err)
	}
	return ids, next, total, nil
}

func (r *PostgresRepository) GetStats(ctx context.Context, listingID string) (Stats, error) {
	var s Stats
	err := r.pool.QueryRow(ctx,
		`SELECT view_count, favorite_count FROM listing_stats WHERE listing_id = $1`,
		listingID).Scan(&s.ViewCount, &s.FavoriteCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Stats{}, nil // no activity yet
		}
		return Stats{}, fmt.Errorf("get stats: %w", err)
	}
	return s, nil
}

// ── Seller follow graph (F1) ──

func (r *PostgresRepository) Follow(ctx context.Context, userID, sellerID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO follows (user_id, seller_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, sellerID)
	if err != nil {
		return false, fmt.Errorf("follow: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) Unfollow(ctx context.Context, userID, sellerID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM follows WHERE user_id = $1 AND seller_id = $2`, userID, sellerID)
	if err != nil {
		return false, fmt.Errorf("unfollow: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) IsFollowing(ctx context.Context, userID, sellerID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE user_id = $1 AND seller_id = $2)`,
		userID, sellerID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is following: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) ListFollowedSellers(ctx context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	limit := clampPageSize(pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT seller_id FROM follows
		 WHERE user_id = $1 AND ($2 = '' OR seller_id > $2)
		 ORDER BY seller_id LIMIT $3`,
		userID, cursor, limit+1)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list followed sellers: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", 0, fmt.Errorf("scan followed seller: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM follows WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count followed sellers: %w", err)
	}
	return ids, next, total, nil
}

func (r *PostgresRepository) ListFollowedListings(ctx context.Context, userID, cursor string, pageSize int32) ([]string, string, int64, error) {
	limit := clampPageSize(pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT sl.listing_id FROM follows f
		 JOIN seller_listings sl ON sl.seller_id = f.seller_id
		 WHERE f.user_id = $1 AND ($2 = '' OR sl.listing_id > $2)
		 ORDER BY sl.listing_id LIMIT $3`,
		userID, cursor, limit+1)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list followed listings: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", 0, fmt.Errorf("scan followed listing: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}

	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(DISTINCT sl.listing_id) FROM follows f
		 JOIN seller_listings sl ON sl.seller_id = f.seller_id
		 WHERE f.user_id = $1`,
		userID).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count followed listings: %w", err)
	}
	return ids, next, total, nil
}

func (r *PostgresRepository) IndexSellerListing(ctx context.Context, sellerID, listingID string) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO seller_listings (seller_id, listing_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		sellerID, listingID); err != nil {
		return fmt.Errorf("index seller listing: %w", err)
	}
	return nil
}

// ── Loyalty / daily check-in (F4) ──

func (r *PostgresRepository) CheckIn(ctx context.Context, userID string, day time.Time) (Loyalty, int64, error) {
	d := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Loyalty{}, 0, fmt.Errorf("check-in begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Claim today's check-in; ON CONFLICT DO NOTHING makes it idempotent per day.
	tag, err := tx.Exec(ctx,
		`INSERT INTO checkins (user_id, day) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, d)
	if err != nil {
		return Loyalty{}, 0, fmt.Errorf("record check-in: %w", err)
	}

	// Load the current account (absent = zero-value snapshot).
	cur, err := loadLoyaltyTx(ctx, tx, userID)
	if err != nil {
		return Loyalty{}, 0, err
	}

	// Already checked in today: no reward, return current snapshot.
	if tag.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return Loyalty{}, 0, fmt.Errorf("check-in commit: %w", err)
		}
		return cur, 0, nil
	}

	newStreak := int32(1)
	if !cur.LastCheckin.IsZero() && cur.LastCheckin.Equal(d.AddDate(0, 0, -1)) {
		newStreak = cur.Streak + 1
	}
	next := Loyalty{
		Streak:      newStreak,
		CoinBalance: cur.CoinBalance + CheckInCoins,
		LastCheckin: d,
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO loyalty_accounts (user_id, coin_balance, streak, last_checkin)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE
		   SET coin_balance = $2, streak = $3, last_checkin = $4`,
		userID, next.CoinBalance, next.Streak, d); err != nil {
		return Loyalty{}, 0, fmt.Errorf("upsert loyalty account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Loyalty{}, 0, fmt.Errorf("check-in commit: %w", err)
	}
	return next, CheckInCoins, nil
}

func (r *PostgresRepository) GetLoyalty(ctx context.Context, userID string) (Loyalty, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT coin_balance, streak, last_checkin FROM loyalty_accounts WHERE user_id = $1`,
		userID)
	return scanLoyalty(row)
}

// loyaltyRow is the subset of pgx.Row / QueryRow both the pool and a tx satisfy.
type loyaltyRow interface {
	Scan(dest ...any) error
}

func loadLoyaltyTx(ctx context.Context, tx pgx.Tx, userID string) (Loyalty, error) {
	row := tx.QueryRow(ctx,
		`SELECT coin_balance, streak, last_checkin FROM loyalty_accounts WHERE user_id = $1`,
		userID)
	return scanLoyalty(row)
}

func scanLoyalty(row loyaltyRow) (Loyalty, error) {
	var (
		l           Loyalty
		lastCheckin *time.Time
	)
	if err := row.Scan(&l.CoinBalance, &l.Streak, &lastCheckin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Loyalty{}, nil // no account yet
		}
		return Loyalty{}, fmt.Errorf("get loyalty: %w", err)
	}
	if lastCheckin != nil {
		l.LastCheckin = dayOf(*lastCheckin)
	}
	return l, nil
}

var _ Repository = (*PostgresRepository)(nil)
