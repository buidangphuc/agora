package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlashHoldTTL is how long an uncommitted hold keeps its units.
const FlashHoldTTL = 15 * time.Minute

// Hold statuses.
const (
	FlashHoldReserved  = "reserved"
	FlashHoldCommitted = "committed"
	FlashHoldReleased  = "released"
)

var (
	// ErrFlashHoldNotFound is returned when a reservation id has no hold row.
	ErrFlashHoldNotFound = errors.New("flash sale hold not found")
	// ErrFlashStockInsufficient is returned when a reserve asks for more units than remain.
	ErrFlashStockInsufficient = errors.New("flash sale stock insufficient")
)

// FlashHold is the persistence view of one stock hold.
type FlashHold struct {
	ReservationID string
	CampaignID    string
	ListingID     string
	VariantID     string
	Quantity      int64
	Status        string
	ExpiresAt     time.Time
	ReleasedAt    time.Time
}

// FlashHoldRepository is the persistence seam for flash-sale stock holds. Time is
// always passed in, so expiry is evaluated at read.
type FlashHoldRepository interface {
	// Reserve creates the hold (ExpiresAt = now+FlashHoldTTL) under the campaign
	// lock, or returns the stored hold with created=false for a known reservation
	// id. ErrCampaignNotFound / ErrFlashStockInsufficient on refusal.
	Reserve(ctx context.Context, h FlashHold, now time.Time) (stored FlashHold, created bool, err error)
	// Get returns the raw row (ErrFlashHoldNotFound on miss).
	Get(ctx context.Context, reservationID string) (FlashHold, error)
	// Commit adds a live hold's quantity to stock_sold. Idempotent for a committed
	// hold; false for an unknown, released or expired one.
	Commit(ctx context.Context, reservationID string, now time.Time) (bool, error)
	// Release returns the units: a committed hold decrements stock_sold once, an
	// active or expired hold changes no counter. found=false for an unknown id.
	Release(ctx context.Context, reservationID string, now time.Time) (found bool, err error)
	// ActiveHeld sums the quantity of reserved, unexpired holds of a campaign.
	ActiveHeld(ctx context.Context, campaignID string, now time.Time) (int64, error)
	// PurgeReleasedAndExpiredHolds deletes released holds released before, and
	// uncommitted holds that expired before. Committed and active holds are kept.
	PurgeReleasedAndExpiredHolds(ctx context.Context, before time.Time) (int64, error)
}

// ── In-memory implementation ──

// InMemoryFlashHoldRepository shares the campaign repository's mutex, so a hold
// and the stock_sold counter always change together.
type InMemoryFlashHoldRepository struct {
	camp  *InMemoryFlashSaleRepository
	holds map[string]FlashHold
}

func NewInMemoryFlashHoldRepository(c *InMemoryFlashSaleRepository) *InMemoryFlashHoldRepository {
	return &InMemoryFlashHoldRepository{camp: c, holds: make(map[string]FlashHold)}
}

func (r *InMemoryFlashHoldRepository) heldLocked(campaignID string, now time.Time) int64 {
	var n int64
	for _, h := range r.holds {
		if h.CampaignID == campaignID && h.Status == FlashHoldReserved && now.Before(h.ExpiresAt) {
			n += h.Quantity
		}
	}
	return n
}

func (r *InMemoryFlashHoldRepository) Reserve(_ context.Context, h FlashHold, now time.Time) (FlashHold, bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	c, ok := r.camp.campaigns[h.CampaignID]
	if !ok {
		return FlashHold{}, false, ErrCampaignNotFound
	}
	if stored, ok := r.holds[h.ReservationID]; ok {
		return stored, false, nil
	}
	if h.Quantity > c.StockCap-c.StockSold-r.heldLocked(c.ID, now) {
		return FlashHold{}, false, ErrFlashStockInsufficient
	}
	h.Status = FlashHoldReserved
	h.ExpiresAt = now.Add(FlashHoldTTL)
	r.holds[h.ReservationID] = h
	return h, true, nil
}

func (r *InMemoryFlashHoldRepository) Get(_ context.Context, id string) (FlashHold, error) {
	r.camp.mu.RLock()
	defer r.camp.mu.RUnlock()
	h, ok := r.holds[id]
	if !ok {
		return FlashHold{}, ErrFlashHoldNotFound
	}
	return h, nil
}

func (r *InMemoryFlashHoldRepository) Commit(_ context.Context, id string, now time.Time) (bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	h, ok := r.holds[id]
	if !ok {
		return false, nil
	}
	switch h.Status {
	case FlashHoldCommitted:
		return true, nil
	case FlashHoldReleased:
		return false, nil
	}
	if !now.Before(h.ExpiresAt) {
		return false, nil
	}
	c := r.camp.campaigns[h.CampaignID]
	c.StockSold += h.Quantity
	r.camp.campaigns[h.CampaignID] = c
	h.Status = FlashHoldCommitted
	r.holds[id] = h
	return true, nil
}

func (r *InMemoryFlashHoldRepository) Release(_ context.Context, id string, now time.Time) (bool, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	h, ok := r.holds[id]
	if !ok {
		return false, nil
	}
	if h.Status == FlashHoldReleased {
		return true, nil
	}
	if h.Status == FlashHoldCommitted {
		c := r.camp.campaigns[h.CampaignID]
		c.StockSold -= h.Quantity
		r.camp.campaigns[h.CampaignID] = c
	}
	h.Status = FlashHoldReleased
	h.ReleasedAt = now
	r.holds[id] = h
	return true, nil
}

func (r *InMemoryFlashHoldRepository) ActiveHeld(_ context.Context, campaignID string, now time.Time) (int64, error) {
	r.camp.mu.RLock()
	defer r.camp.mu.RUnlock()
	return r.heldLocked(campaignID, now), nil
}

func (r *InMemoryFlashHoldRepository) PurgeReleasedAndExpiredHolds(_ context.Context, before time.Time) (int64, error) {
	r.camp.mu.Lock()
	defer r.camp.mu.Unlock()
	var n int64
	for id, h := range r.holds {
		if (h.Status == FlashHoldReleased && h.ReleasedAt.Before(before)) ||
			(h.Status == FlashHoldReserved && h.ExpiresAt.Before(before)) {
			delete(r.holds, id)
			n++
		}
	}
	return n, nil
}

// ── Postgres implementation ──

// PostgresFlashHoldRepository persists holds in promotion_db. Every mutation locks
// the campaign row (FOR UPDATE) first, so reserves of one campaign are serialized
// and the cap cannot be oversold.
type PostgresFlashHoldRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresFlashHoldRepository(pool *pgxpool.Pool) *PostgresFlashHoldRepository {
	return &PostgresFlashHoldRepository{pool: pool}
}

const holdColumns = `reservation_id, campaign_id, listing_id, variant_id, quantity, status, expires_at, released_at`

func scanHold(row pgx.Row, h *FlashHold) error {
	var released *time.Time
	if err := row.Scan(&h.ReservationID, &h.CampaignID, &h.ListingID, &h.VariantID, &h.Quantity, &h.Status, &h.ExpiresAt, &released); err != nil {
		return err
	}
	if released != nil {
		h.ReleasedAt = *released
	}
	return nil
}

func (r *PostgresFlashHoldRepository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *PostgresFlashHoldRepository) Reserve(ctx context.Context, h FlashHold, now time.Time) (FlashHold, bool, error) {
	var stored FlashHold
	var created bool
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var capacity, sold int64
		if err := tx.QueryRow(ctx,
			`SELECT stock_cap, stock_sold FROM flash_sale_campaigns WHERE id = $1 FOR UPDATE`, h.CampaignID,
		).Scan(&capacity, &sold); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCampaignNotFound
			}
			return fmt.Errorf("lock campaign %q: %w", h.CampaignID, err)
		}
		err := scanHold(tx.QueryRow(ctx, `SELECT `+holdColumns+` FROM flash_sale_holds WHERE reservation_id = $1`, h.ReservationID), &stored)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("get hold: %w", err)
		}
		var held int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(SUM(quantity), 0) FROM flash_sale_holds
			 WHERE campaign_id = $1 AND status = $2 AND expires_at > $3`,
			h.CampaignID, FlashHoldReserved, now,
		).Scan(&held); err != nil {
			return fmt.Errorf("sum holds: %w", err)
		}
		if h.Quantity > capacity-sold-held {
			return ErrFlashStockInsufficient
		}
		h.Status = FlashHoldReserved
		h.ExpiresAt = now.Add(FlashHoldTTL)
		if _, err := tx.Exec(ctx,
			`INSERT INTO flash_sale_holds (reservation_id, campaign_id, listing_id, variant_id, quantity, status, expires_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
			h.ReservationID, h.CampaignID, h.ListingID, h.VariantID, h.Quantity, h.Status, h.ExpiresAt, now,
		); err != nil {
			return fmt.Errorf("insert hold: %w", err)
		}
		stored, created = h, true
		return nil
	})
	if err != nil {
		return FlashHold{}, false, err
	}
	return stored, created, nil
}

func (r *PostgresFlashHoldRepository) Get(ctx context.Context, id string) (FlashHold, error) {
	var h FlashHold
	if err := scanHold(r.pool.QueryRow(ctx, `SELECT `+holdColumns+` FROM flash_sale_holds WHERE reservation_id = $1`, id), &h); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlashHold{}, ErrFlashHoldNotFound
		}
		return FlashHold{}, fmt.Errorf("get hold %q: %w", id, err)
	}
	return h, nil
}

// lockHold locks the campaign row, then the hold row (same order as Reserve, so no
// deadlock) and returns the hold; found=false when the reservation is unknown.
func (r *PostgresFlashHoldRepository) lockHold(ctx context.Context, tx pgx.Tx, id string) (FlashHold, bool, error) {
	var campaignID string
	if err := tx.QueryRow(ctx, `SELECT campaign_id FROM flash_sale_holds WHERE reservation_id = $1`, id).Scan(&campaignID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlashHold{}, false, nil
		}
		return FlashHold{}, false, fmt.Errorf("find hold %q: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM flash_sale_campaigns WHERE id = $1 FOR UPDATE`, campaignID); err != nil {
		return FlashHold{}, false, fmt.Errorf("lock campaign %q: %w", campaignID, err)
	}
	var h FlashHold
	if err := scanHold(tx.QueryRow(ctx, `SELECT `+holdColumns+` FROM flash_sale_holds WHERE reservation_id = $1 FOR UPDATE`, id), &h); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlashHold{}, false, nil
		}
		return FlashHold{}, false, fmt.Errorf("lock hold %q: %w", id, err)
	}
	return h, true, nil
}

func (r *PostgresFlashHoldRepository) Commit(ctx context.Context, id string, now time.Time) (bool, error) {
	var ok bool
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		h, found, err := r.lockHold(ctx, tx, id)
		if err != nil || !found {
			return err
		}
		switch h.Status {
		case FlashHoldCommitted:
			ok = true
			return nil
		case FlashHoldReleased:
			return nil
		}
		if !now.Before(h.ExpiresAt) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE flash_sale_campaigns SET stock_sold = stock_sold + $2, updated_at = $3 WHERE id = $1`, h.CampaignID, h.Quantity, now); err != nil {
			return fmt.Errorf("increment stock_sold: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE flash_sale_holds SET status = $2, updated_at = $3 WHERE reservation_id = $1`, id, FlashHoldCommitted, now); err != nil {
			return fmt.Errorf("commit hold: %w", err)
		}
		ok = true
		return nil
	})
	return ok, err
}

func (r *PostgresFlashHoldRepository) Release(ctx context.Context, id string, now time.Time) (bool, error) {
	var ok bool
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		h, found, err := r.lockHold(ctx, tx, id)
		if err != nil || !found {
			return err
		}
		ok = true
		if h.Status == FlashHoldReleased {
			return nil
		}
		if h.Status == FlashHoldCommitted {
			if _, err := tx.Exec(ctx, `UPDATE flash_sale_campaigns SET stock_sold = stock_sold - $2, updated_at = $3 WHERE id = $1`, h.CampaignID, h.Quantity, now); err != nil {
				return fmt.Errorf("decrement stock_sold: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE flash_sale_holds SET status = $2, released_at = $3, updated_at = $3 WHERE reservation_id = $1`, id, FlashHoldReleased, now); err != nil {
			return fmt.Errorf("release hold: %w", err)
		}
		return nil
	})
	return ok, err
}

func (r *PostgresFlashHoldRepository) ActiveHeld(ctx context.Context, campaignID string, now time.Time) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity), 0) FROM flash_sale_holds WHERE campaign_id = $1 AND status = $2 AND expires_at > $3`,
		campaignID, FlashHoldReserved, now,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("sum active holds: %w", err)
	}
	return n, nil
}

func (r *PostgresFlashHoldRepository) PurgeReleasedAndExpiredHolds(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM flash_sale_holds
		 WHERE (status = $2 AND released_at < $1) OR (status = $3 AND expires_at < $1)`,
		before, FlashHoldReleased, FlashHoldReserved)
	if err != nil {
		return 0, fmt.Errorf("purge holds: %w", err)
	}
	return tag.RowsAffected(), nil
}
