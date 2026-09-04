// Package service holds business logic, independent of gRPC and of storage. It
// depends on the repository port and returns domain values; the handler
// (internal/handler) adapts these to the generated protobuf types.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// ErrForbidden is returned when a caller tries to modify a listing they do not
// own. The handler maps it to codes.PermissionDenied.
var ErrForbidden = errors.New("not the listing owner")

// DefaultReservationTTL bounds how long a stock reservation is held before the
// sweeper releases it (AD3). A checkout that crashes before confirming frees its
// reserved stock after this window instead of leaking it.
const DefaultReservationTTL = 15 * time.Minute

type ListingService struct {
	repo repository.ListingRepository
	// tx is the transactional writer used by the *WithEvent paths: it persists
	// the listing write and its outbox event in one transaction. When nil (e.g.
	// unit tests with only an in-memory repo), the *WithEvent methods fall back
	// to a plain repo write with no event — matching the prior no-emit behaviour.
	tx repository.TxWriter
}

func NewListingService(repo repository.ListingRepository) *ListingService {
	return &ListingService{repo: repo}
}

// NewListingServiceWithOutbox wires the transactional writer so every write also
// records its domain event in the same DB transaction (the outbox).
func NewListingServiceWithOutbox(repo repository.ListingRepository, tx repository.TxWriter) *ListingService {
	return &ListingService{repo: repo, tx: tx}
}

// Get returns a single listing by id.
func (s *ListingService) Get(ctx context.Context, id string) (repository.Listing, error) {
	return s.repo.Get(ctx, id)
}

// List returns a keyset page of listings, optionally filtered by status.
func (s *ListingService) List(ctx context.Context, cursor string, pageSize int32, status string) (repository.Page, error) {
	return s.repo.List(ctx, cursor, pageSize, status)
}

// ListMine returns a page of the owner's own listings (the seller center).
func (s *ListingService) ListMine(ctx context.Context, ownerID, cursor string, pageSize int32) (repository.Page, error) {
	return s.repo.ListByOwner(ctx, ownerID, cursor, pageSize)
}

// Create stores a new listing owned by ownerID, assigning a server-side id when
// the caller left it empty. The handler emits ChangeType_CREATED on success.
func (s *ListingService) Create(ctx context.Context, l repository.Listing, ownerID string) (repository.Listing, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.SellerID = ownerID
	return s.repo.Create(ctx, l)
}

// Update replaces an existing listing. Only the owner (or an admin) may update
// it; the stored owner is preserved. The handler emits ChangeType_UPDATED.
func (s *ListingService) Update(ctx context.Context, l repository.Listing, ownerID string, isAdmin bool) (repository.Listing, error) {
	existing, err := s.repo.Get(ctx, l.ID)
	if err != nil {
		return repository.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return repository.Listing{}, ErrForbidden
	}
	l.SellerID = existing.SellerID // owner is immutable
	return s.repo.Update(ctx, l)
}

// Delete removes a listing the caller owns (or any, for admin). The handler
// emits ChangeType_DELETED with the removed listing.
func (s *ListingService) Delete(ctx context.Context, id, ownerID string, isAdmin bool) (repository.Listing, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return repository.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return repository.Listing{}, ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}

// CreateWithEvent stores a new listing and records its domain event in the same
// transaction. It assigns a server-side id when the caller left one empty and
// sets the owner, then hands the persisted listing to enqueue (which builds the
// EventEnvelope) inside the write tx. When no transactional writer is wired it
// falls back to a plain create with no event.
func (s *ListingService) CreateWithEvent(ctx context.Context, l repository.Listing, ownerID string, enqueue repository.EnqueueFn) (repository.Listing, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.SellerID = ownerID
	if s.tx == nil {
		return s.repo.Create(ctx, l)
	}
	return s.tx.CreateTx(ctx, l, enqueue)
}

// UpdateWithEvent replaces an existing listing (owner or admin only) and records
// its domain event in the same transaction. The stored owner is preserved.
func (s *ListingService) UpdateWithEvent(ctx context.Context, l repository.Listing, ownerID string, isAdmin bool, enqueue repository.EnqueueFn) (repository.Listing, error) {
	existing, err := s.repo.Get(ctx, l.ID)
	if err != nil {
		return repository.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return repository.Listing{}, ErrForbidden
	}
	l.SellerID = existing.SellerID // owner is immutable
	if s.tx == nil {
		return s.repo.Update(ctx, l)
	}
	return s.tx.UpdateTx(ctx, l, enqueue)
}

// DeleteWithEvent removes a listing the caller owns (or any, for admin) and
// records its domain event in the same transaction.
func (s *ListingService) DeleteWithEvent(ctx context.Context, id, ownerID string, isAdmin bool, enqueue repository.EnqueueFn) (repository.Listing, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return repository.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return repository.Listing{}, ErrForbidden
	}
	if s.tx == nil {
		return s.repo.Delete(ctx, id)
	}
	return s.tx.DeleteTx(ctx, id, enqueue)
}

// ReserveStock delegates to repository.ReserveStock.
func (s *ListingService) ReserveStock(ctx context.Context, listingID, variantID string, quantity int32) error {
	return s.repo.ReserveStock(ctx, listingID, variantID, quantity)
}

// ReleaseStock delegates to repository.ReleaseStock.
func (s *ListingService) ReleaseStock(ctx context.Context, listingID, variantID string, quantity int32) error {
	return s.repo.ReleaseStock(ctx, listingID, variantID, quantity)
}

// ReserveStockIdempotent reserves stock keyed on a stable reservationID so a
// retried checkout decrements exactly once (AD5). The reservation is held for
// DefaultReservationTTL, after which SweepExpiredReservations restores it. The
// caller supplies reservationID (stable per cart-item + attempt); an empty id
// falls back to a plain, non-idempotent reserve.
func (s *ListingService) ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32) error {
	return s.repo.ReserveStockIdempotent(ctx, reservationID, listingID, variantID, quantity, time.Now().Add(DefaultReservationTTL))
}

// SweepExpiredReservations releases reservations past their TTL and restores the
// reserved stock (AD3 domain side), returning how many were released. It is
// idempotent and safe to run on a periodic schedule (see ReservationSweeper).
func (s *ListingService) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	return s.repo.SweepExpiredReservations(ctx, now)
}

