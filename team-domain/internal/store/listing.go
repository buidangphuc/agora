// Package store is the persistence boundary. It exposes domain types and a
// port; the concrete store is swappable (Postgres in production, an in-memory
// fake for tests). Nothing above this package imports a DB driver directly.
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Get/Update/Delete when the row is absent.
var (
	ErrNotFound        = errors.New("listing not found")
	ErrOutOfStock      = errors.New("insufficient stock")
	ErrVariantNotFound = errors.New("variant not found")

	// ErrReservationNotFound: the reservation_id is unknown (CommitReservation).
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrReservationReleased: the reservation was already released (its stock
	// was given back). CommitReservation and a re-reserve under the same id fail
	// with it instead of reporting a false success.
	ErrReservationReleased = errors.New("reservation already released")
	// ErrReservationMismatch: a reserve repeated an existing reservation_id with
	// a different listing, variant or quantity. A retry must be identical.
	ErrReservationMismatch = errors.New("reservation id reused with different listing, variant or quantity")
	// ErrReservationIDRequired: ReserveStockIdempotent was called without a
	// reservation id. There is no ledger-less reserve path.
	ErrReservationIDRequired = errors.New("reservation id required")
	// ErrListingNotReservable: a new reservation targeted a held listing.
	ErrListingNotReservable = errors.New("listing not reservable")
)

// Reservation lifecycle (owned by team-domain): active -> committed | released,
// and committed -> released. Only an active reservation expires (TTL sweep);
// stock is restored once, when a reservation becomes released.
const (
	ReservationActive    = "active"
	ReservationCommitted = "committed"
	ReservationReleased  = "released"
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
	Price       int64     // minor units (e.g. VND), integer to avoid float money
	Currency    string    // ISO 4217
	Status      string    // "draft" | "published" | "rejected"
	SellerID    string    // owner Principal id (server-assigned on create)
	ImageKeys   []string  // storage keys in object store
	CategoryID  string    // category ID
	Stock       int32     // base stock if no variants
	Variants    []Variant // product variants

	// Moderation state (columns added by migration 0011).
	ModerationReasonCode string
	ModerationNote       string
	ScreenFlags          []string
	ModerationDecidedAt  time.Time
	SubmittedAt          time.Time

	// Optimistic-concurrency fields. UpdatedAt is filled by
	// Get/scan from listings.updated_at. For UpdateTx, LoadedStatus and
	// LoadedUpdatedAt are the status/updated_at the caller read before deciding;
	// when LoadedUpdatedAt is non-zero the write is conditional on both.
	UpdatedAt       time.Time
	LoadedStatus    string
	LoadedUpdatedAt time.Time
}

// ErrStatusChanged: a conditional write (UpdateTx with LoadedUpdatedAt, or
// ModerateTx) found the row's status/updated_at no longer what the caller read.
var ErrStatusChanged = errors.New("listing changed, reload and retry")

// ModerationUpdate is the admin decision persisted by ModerateTx.
type ModerationUpdate struct {
	ToStatus   string
	ReasonCode string
	Note       string
	DecidedAt  time.Time
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
	// ReserveStockIdempotent decrements inventory keyed on a stable reservationID:
	// a repeat call with the id of an active or committed reservation is a no-op
	// returning nil, so a retried checkout never double-decrements; a repeat with
	// the id of a RELEASED reservation returns ErrReservationReleased (its stock
	// was given back, the caller must use a new id). The reservation is recorded
	// with the given expiresAt. reservationID is required
	// (ErrReservationIDRequired): there is no ledger-less decrement, so every
	// decrement can be committed, released and swept.
	ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32, expiresAt time.Time) error
	// SweepExpiredReservations releases every still-active reservation whose
	// expires_at is at or before now, restoring the reserved stock, and returns how
	// many were released (AD3 domain side). Idempotent and safe to re-run.
	SweepExpiredReservations(ctx context.Context, now time.Time) (int, error)
	// CommitReservation moves an active reservation to committed so the TTL
	// sweeper never restores it. Idempotent: an already committed reservation is
	// a no-op success. ErrReservationReleased if it was released;
	// ErrReservationNotFound if the id is unknown.
	CommitReservation(ctx context.Context, reservationID string) error
	// ReleaseReservation releases a reservation by id: if it is active or
	// committed it moves to released and the quantity STORED on the reservation
	// is restored, exactly once, in the same transaction. A repeat, or a
	// reservation the sweep already released, is ReleaseNoOp; an id that was
	// never reserved is ReleaseUnknown (also a no-op, reported separately so the
	// caller can log it). The caller-supplied quantity is never used.
	ReleaseReservation(ctx context.Context, reservationID string) (ReleaseOutcome, error)
}

// ReleaseOutcome reports what ReleaseReservation did.
type ReleaseOutcome int

const (
	// ReleaseApplied: the reservation moved to released and stock was restored.
	ReleaseApplied ReleaseOutcome = iota
	// ReleaseNoOp: the reservation was already released (or swept); nothing changed.
	ReleaseNoOp
	// ReleaseUnknown: no reservation with that id exists; nothing changed.
	ReleaseUnknown
)
