// Package repository is the persistence boundary. It exposes domain types and a
// port; the concrete store is swappable (Postgres in production, an in-memory
// fake for tests). Nothing above this package imports a DB driver directly.
package repository

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrNotFound is returned by Get/Update/Delete when the row is absent.
var (
	ErrNotFound        = errors.New("listing not found")
	ErrOutOfStock      = errors.New("insufficient stock")
	ErrVariantNotFound = errors.New("variant not found")
)

// Page sizing: a page request of 0 uses the default; anything above the max is
// clamped so a caller can't ask for an unbounded scan.
const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// Variant represents a specific SKU/option of a product.
type Variant struct {
	ID        string
	ListingID string
	Name      string
	SKU       string
	Price     int64
	Stock     int32
	ImageURL  string
}

// Listing is the persistence-layer domain value. It is intentionally decoupled
// from the generated protobuf Listing: the handler maps between wire and domain
// so the contract and the storage model can evolve independently.
type Listing struct {
	ID          string
	Title       string
	Description string
	Price       int64    // minor units (e.g. VND), integer to avoid float money
	Currency    string   // ISO 4217
	Status      string   // "draft" | "published" | "rejected"
	SellerID    string   // owner Principal id (server-assigned on create)
	ImageKeys   []string // storage keys in object store
	CategoryID  string   // category ID
	Stock       int32    // base stock if no variants
	Variants    []Variant // product variants
}

// Page is a single page of listings plus keyset-pagination metadata.
type Page struct {
	Items      []Listing
	NextCursor string // empty => no further pages
	Total      int64  // total rows matching the query (-1 => unknown)
}

// ListingRepository is the port the service depends on. Cursor is an opaque
// keyset (the last id of the previous page); empty means "from the start".
type ListingRepository interface {
	Get(ctx context.Context, id string) (Listing, error)
	// List returns a page filtered by status (empty status = all).
	List(ctx context.Context, cursor string, pageSize int32, status string) (Page, error)
	// ListByOwner returns a page of listings owned by ownerID.
	ListByOwner(ctx context.Context, ownerID, cursor string, pageSize int32) (Page, error)
	// Create inserts a new listing (id must be set by the caller).
	Create(ctx context.Context, l Listing) (Listing, error)
	// Update replaces the listing identified by l.ID; ErrNotFound if absent.
	Update(ctx context.Context, l Listing) (Listing, error)
	// Delete removes the listing by id, returning the deleted row (for the
	// event); ErrNotFound if absent.
	Delete(ctx context.Context, id string) (Listing, error)
	// ReserveStock atomically decrements inventory if sufficient stock is available.
	ReserveStock(ctx context.Context, listingID, variantID string, quantity int32) error
	// ReleaseStock releases previously reserved inventory back into stock.
	ReleaseStock(ctx context.Context, listingID, variantID string, quantity int32) error
	// ReserveStockIdempotent decrements inventory keyed on a stable reservationID
	// (AD5): a repeat call with the same id finds the prior reservation and is a
	// no-op returning nil, so a retried checkout never double-decrements. The
	// reservation is recorded with the given expiresAt TTL; an empty reservationID
	// falls back to a plain (non-idempotent) ReserveStock.
	ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32, expiresAt time.Time) error
	// SweepExpiredReservations releases every still-active reservation whose
	// expires_at is at or before now, restoring the reserved stock, and returns how
	// many were released (AD3 domain side). Idempotent and safe to re-run.
	SweepExpiredReservations(ctx context.Context, now time.Time) (int, error)
}

// clampPageSize normalizes a requested page size into [1, MaxPageSize].
func clampPageSize(pageSize int32) int {
	switch {
	case pageSize <= 0:
		return DefaultPageSize
	case int(pageSize) > MaxPageSize:
		return MaxPageSize
	default:
		return int(pageSize)
	}
}

// InMemoryListingRepository is a deterministic fake store for tests. Production
// uses PostgresListingRepository (listing_pg.go).
type InMemoryListingRepository struct {
	mu   sync.RWMutex
	byID map[string]Listing
	// reservations records idempotent reservations keyed by reservation_id, so a
	// re-reserve with the same id is a no-op and the sweeper can restore expired
	// holds. It mirrors the Postgres `reservations` table.
	reservations map[string]memReservation
}

// memReservation is the in-memory analogue of one `reservations` row.
type memReservation struct {
	listingID string
	variantID string
	quantity  int32
	expiresAt time.Time
	released  bool
}

// NewInMemoryListingRepository returns a store seeded with the given rows (or a
// single default row when none are provided).
func NewInMemoryListingRepository(seed ...Listing) *InMemoryListingRepository {
	if len(seed) == 0 {
		seed = []Listing{{
			ID:          "seed-1",
			Title:       "Seed listing",
			Description: "In-memory row proving the read path end-to-end.",
			Price:       1_500_000_000,
			Currency:    "VND",
			Status:      "published",
			Stock:       100,
		}}
	}
	byID := make(map[string]Listing, len(seed))
	for _, l := range seed {
		byID[l.ID] = l
	}
	return &InMemoryListingRepository{byID: byID, reservations: map[string]memReservation{}}
}

// Get returns the listing for id, or ErrNotFound.
func (r *InMemoryListingRepository) Get(_ context.Context, id string) (Listing, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.byID[id]
	if !ok {
		return Listing{}, ErrNotFound
	}
	return l, nil
}

// List returns a keyset page ordered by id, filtered by status.
func (r *InMemoryListingRepository) List(_ context.Context, cursor string, pageSize int32, status string) (Page, error) {
	return r.page(cursor, pageSize, func(l Listing) bool {
		return status == "" || l.Status == status
	}), nil
}

// ListByOwner returns a keyset page of the owner's listings.
func (r *InMemoryListingRepository) ListByOwner(_ context.Context, ownerID, cursor string, pageSize int32) (Page, error) {
	return r.page(cursor, pageSize, func(l Listing) bool {
		return l.SellerID == ownerID
	}), nil
}

// page is the shared keyset-pagination + predicate filter over the fake store.
func (r *InMemoryListingRepository) page(cursor string, pageSize int32, keep func(Listing) bool) Page {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.byID))
	var total int64
	for id, l := range r.byID {
		if keep(l) {
			ids = append(ids, id)
			total++
		}
	}
	sort.Strings(ids)

	limit := clampPageSize(pageSize)
	items := make([]Listing, 0, limit)
	var next string
	for _, id := range ids {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(items) == limit {
			next = items[len(items)-1].ID
			break
		}
		items = append(items, r.byID[id])
	}
	return Page{Items: items, NextCursor: next, Total: total}
}

// Create inserts a new listing (overwrites on duplicate id in this fake).
func (r *InMemoryListingRepository) Create(_ context.Context, l Listing) (Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[l.ID] = l
	return l, nil
}

// Update replaces an existing listing, or returns ErrNotFound.
func (r *InMemoryListingRepository) Update(_ context.Context, l Listing) (Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[l.ID]; !ok {
		return Listing{}, ErrNotFound
	}
	r.byID[l.ID] = l
	return l, nil
}

// Delete removes a listing, returning the deleted row or ErrNotFound.
func (r *InMemoryListingRepository) Delete(_ context.Context, id string) (Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.byID[id]
	if !ok {
		return Listing{}, ErrNotFound
	}
	delete(r.byID, id)
	return l, nil
}

// ReserveStock reserves inventory in memory.
func (r *InMemoryListingRepository) ReserveStock(_ context.Context, listingID, variantID string, quantity int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reserveLocked(listingID, variantID, quantity)
}

// ReleaseStock releases inventory in memory.
func (r *InMemoryListingRepository) ReleaseStock(_ context.Context, listingID, variantID string, quantity int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.releaseLocked(listingID, variantID, quantity)
}

// reserveLocked decrements stock; the caller must hold r.mu.
func (r *InMemoryListingRepository) reserveLocked(listingID, variantID string, quantity int32) error {
	l, ok := r.byID[listingID]
	if !ok {
		return ErrNotFound
	}
	if variantID == "" {
		if l.Stock < quantity {
			return ErrOutOfStock
		}
		l.Stock -= quantity
		r.byID[listingID] = l
		return nil
	}
	for i := range l.Variants {
		if l.Variants[i].ID == variantID {
			if l.Variants[i].Stock < quantity {
				return ErrOutOfStock
			}
			l.Variants[i].Stock -= quantity
			r.byID[listingID] = l
			return nil
		}
	}
	return ErrVariantNotFound
}

// releaseLocked restores stock; the caller must hold r.mu.
func (r *InMemoryListingRepository) releaseLocked(listingID, variantID string, quantity int32) error {
	l, ok := r.byID[listingID]
	if !ok {
		return ErrNotFound
	}
	if variantID == "" {
		l.Stock += quantity
		r.byID[listingID] = l
		return nil
	}
	for i := range l.Variants {
		if l.Variants[i].ID == variantID {
			l.Variants[i].Stock += quantity
			r.byID[listingID] = l
			return nil
		}
	}
	return ErrVariantNotFound
}

// ReserveStockIdempotent reserves stock at most once per reservationID (AD5): a
// repeat call with an already-applied id returns nil without touching stock.
func (r *InMemoryListingRepository) ReserveStockIdempotent(_ context.Context, reservationID, listingID, variantID string, quantity int32, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservationID != "" {
		if _, ok := r.reservations[reservationID]; ok {
			return nil // already applied: prior result was success
		}
	}
	if err := r.reserveLocked(listingID, variantID, quantity); err != nil {
		return err
	}
	if reservationID != "" {
		r.reservations[reservationID] = memReservation{
			listingID: listingID,
			variantID: variantID,
			quantity:  quantity,
			expiresAt: expiresAt,
		}
	}
	return nil
}

// SweepExpiredReservations restores stock for every active reservation whose TTL
// has passed and marks it released (AD3). Re-running is a no-op for already
// released rows.
func (r *InMemoryListingRepository) SweepExpiredReservations(_ context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	released := 0
	for id, res := range r.reservations {
		if res.released || res.expiresAt.After(now) {
			continue
		}
		// Best-effort restore: a since-deleted listing is still marked released so
		// the sweep stays idempotent and never loops on it.
		_ = r.releaseLocked(res.listingID, res.variantID, res.quantity)
		res.released = true
		r.reservations[id] = res
		released++
	}
	return released, nil
}

// compile-time assertion that the fake satisfies the port.
var _ ListingRepository = (*InMemoryListingRepository)(nil)
