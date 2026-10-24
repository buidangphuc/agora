// Package listing holds business logic, independent of gRPC and of storage. It
// depends on the repository port and returns domain values; the handler
// (internal/handler) adapts these to the generated protobuf types.
package listing

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
	"github.com/buidangphuc/team-domain/internal/screen"
	"github.com/buidangphuc/team-domain/internal/store"
)

// ErrStatusLocked: the listing is BANNED or HELD; seller writes and deletes are
// refused. The handler maps it to codes.FailedPrecondition.
var ErrStatusLocked = errors.New("listing status is locked")

// KeywordMatcher is the local prohibited-term screen (*screen.Screener satisfies it).
type KeywordMatcher interface {
	Match(text string) []string
}

// Screener is the AI screen (*screen.AIClient satisfies it).
type Screener interface {
	ScreenListing(ctx context.Context, in screen.ScreenInput) (*aiv1.ScreenListingResponse, error)
}

// ErrForbidden is returned when a caller tries to modify a listing they do not
// own. The handler maps it to codes.PermissionDenied.
var ErrForbidden = errors.New("not the listing owner")

// DefaultReservationTTL bounds how long a stock reservation is held before the
// sweeper releases it (AD3). A checkout that crashes before confirming frees its
// reserved stock after this window instead of leaking it.
const DefaultReservationTTL = 15 * time.Minute

// DefaultSweepInterval is how often ReservationSweeper runs when no (or a
// non-positive) interval is configured.
const DefaultSweepInterval = time.Minute

type ListingService struct {
	audit *auditx.Gate
	repo  store.ListingRepository
	// tx is the transactional writer used by the *WithEvent paths: it persists
	// the listing write and its outbox event in one transaction. When nil (e.g.
	// unit tests with only an in-memory repo), the *WithEvent methods fall back
	// to a plain repo write with no event — matching the prior no-emit behaviour.
	tx store.TxWriter
	// reservationTTL is the hold time stamped on new reservations; zero means
	// DefaultReservationTTL. Set via WithReservationTTL (RESERVATION_TTL).
	reservationTTL time.Duration
	// kw and ai screen listings on publish / content edit; either may be nil.
	kw KeywordMatcher
	ai Screener
}

// WithScreener wires the keyword matcher and the AI screener (either may be nil;
// a missing screener fails safe to pending_review).
func (s *ListingService) WithScreener(kw KeywordMatcher, ai Screener) *ListingService {
	s.kw = kw
	s.ai = ai
	return s
}

// WithReservationTTL sets how long new reservations hold stock before the
// sweeper may restore them. A non-positive ttl is ignored (the default stays).
// Returns the service for chaining; the constructors are unchanged.
func (s *ListingService) WithReservationTTL(ttl time.Duration) *ListingService {
	if ttl > 0 {
		s.reservationTTL = ttl
	}
	return s
}

// WithAudit attaches the audit gate used for admin updates, deletes and moderation.
func (s *ListingService) WithAudit(a *auditx.Gate) *ListingService {
	s.audit = a
	return s
}

// ReservationTTL returns the effective reservation hold time.
func (s *ListingService) ReservationTTL() time.Duration {
	if s.reservationTTL > 0 {
		return s.reservationTTL
	}
	return DefaultReservationTTL
}

func NewListingService(repo store.ListingRepository) *ListingService {
	return &ListingService{repo: repo, audit: auditx.NewGate(nil, nil)}
}

// NewListingServiceWithOutbox wires the transactional writer so every write also
// records its domain event in the same DB transaction (the outbox).
func NewListingServiceWithOutbox(repo store.ListingRepository, tx store.TxWriter) *ListingService {
	return &ListingService{repo: repo, tx: tx, audit: auditx.NewGate(nil, nil)}
}

// Get returns a single listing by id.
func (s *ListingService) Get(ctx context.Context, id string) (store.Listing, error) {
	return s.repo.Get(ctx, id)
}

// List returns a keyset page of listings, optionally filtered by status.
func (s *ListingService) List(ctx context.Context, cursor string, pageSize int32, status string) (store.Page, error) {
	return s.repo.List(ctx, cursor, pageSize, status)
}

// ListMine returns a page of the owner's own listings (the seller center).
func (s *ListingService) ListMine(ctx context.Context, ownerID, cursor string, pageSize int32) (store.Page, error) {
	return s.repo.ListByOwner(ctx, ownerID, cursor, pageSize)
}

// Create stores a new listing owned by ownerID, assigning a server-side id when
// the caller left it empty. The handler emits ChangeType_CREATED on success.
func (s *ListingService) Create(ctx context.Context, l store.Listing, ownerID string) (store.Listing, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.SellerID = ownerID
	l.Status = createStatus(l.Status)
	return s.repo.Create(ctx, l)
}

// Update replaces an existing listing. Only the owner (or an admin) may update
// it; the stored owner is preserved. The handler emits ChangeType_UPDATED.
func (s *ListingService) Update(ctx context.Context, l store.Listing, ownerID string, isAdmin bool) (store.Listing, error) {
	existing, err := s.repo.Get(ctx, l.ID)
	if err != nil {
		return store.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return store.Listing{}, ErrForbidden
	}
	l.SellerID = existing.SellerID // owner is immutable
	return s.repo.Update(ctx, l)
}

// Delete removes a listing the caller owns (or any, for admin). The handler
// emits ChangeType_DELETED with the removed listing.
func (s *ListingService) Delete(ctx context.Context, id, ownerID string, isAdmin bool) (store.Listing, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return store.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return store.Listing{}, ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}

// CreateWithEvent stores a new listing and records its domain event in the same
// transaction. It assigns a server-side id when the caller left one empty and
// sets the owner, then hands the persisted listing to enqueue (which builds the
// EventEnvelope) inside the write tx. When no transactional writer is wired it
// falls back to a plain create with no event.
func (s *ListingService) CreateWithEvent(ctx context.Context, l store.Listing, ownerID string, enqueue store.EnqueueFn) (store.Listing, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.SellerID = ownerID
	l.Status = createStatus(l.Status)
	if l.Status == "published" {
		s.applyScreen(ctx, &l)
	}
	if s.tx == nil {
		return s.repo.Create(ctx, l)
	}
	return s.tx.CreateTx(ctx, l, enqueue)
}

// UpdateWithEvent replaces an existing listing (owner or admin only) and records
// its domain event in the same transaction. The stored owner is preserved.
func (s *ListingService) UpdateWithEvent(ctx context.Context, l store.Listing, ownerID string, isAdmin bool, enqueue store.EnqueueFn) (store.Listing, error) {
	existing, err := s.repo.Get(ctx, l.ID)
	if err != nil {
		return store.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return store.Listing{}, ErrForbidden
	}
	if !isAdmin {
		if err := s.resolveSellerUpdate(ctx, existing, &l); err != nil {
			return store.Listing{}, err
		}
	}
	l.SellerID = existing.SellerID // owner is immutable
	if !isAdmin {
		return s.writeUpdate(ctx, l, enqueue)
	}
	// Admin cannot set status here: the stored status is kept and audited.
	l.Status = existing.Status
	var updated store.Listing
	ev := auditx.Event{
		ActorID: ownerID, Action: "listing.admin_update", TargetType: "listing", TargetID: existing.ID,
		Metadata: map[string]string{"seller_id": existing.SellerID, "status": strings.ToLower(l.Status)},
	}
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var werr error
		updated, werr = s.writeUpdate(ctx, l, enqueue)
		return werr
	})
	return updated, err
}

func (s *ListingService) writeUpdate(ctx context.Context, l store.Listing, enqueue store.EnqueueFn) (store.Listing, error) {
	if s.tx == nil {
		return s.repo.Update(ctx, l)
	}
	return s.tx.UpdateTx(ctx, l, enqueue)
}

// DeleteWithEvent removes a listing the caller owns (or any, for admin) and
// records its domain event in the same transaction.
func (s *ListingService) DeleteWithEvent(ctx context.Context, id, ownerID string, isAdmin bool, enqueue store.EnqueueFn) (store.Listing, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return store.Listing{}, err
	}
	if existing.SellerID != ownerID && !isAdmin {
		return store.Listing{}, ErrForbidden
	}
	if !isAdmin && isLockedStatus(existing.Status) {
		return store.Listing{}, ErrStatusLocked
	}
	if !isAdmin {
		return s.writeDelete(ctx, id, enqueue)
	}
	var deleted store.Listing
	ev := auditx.Event{
		ActorID: ownerID, Action: "listing.admin_delete", TargetType: "listing", TargetID: existing.ID,
		Metadata: map[string]string{"seller_id": existing.SellerID},
	}
	err = s.audit.Do(ctx, ev, func(ctx context.Context) error {
		var werr error
		deleted, werr = s.writeDelete(ctx, id, enqueue)
		return werr
	})
	return deleted, err
}

func (s *ListingService) writeDelete(ctx context.Context, id string, enqueue store.EnqueueFn) (store.Listing, error) {
	if s.tx == nil {
		return s.repo.Delete(ctx, id)
	}
	return s.tx.DeleteTx(ctx, id, enqueue)
}

// ReleaseStock releases the reservation reservationID: the quantity STORED on it
// is restored exactly once. A repeat, a release of a reservation the sweep
// already returned, and an unknown id are successful no-ops, each logged at WARN
// (a zero-row release is expected on a retry but can also mask a caller bug).
// The handler rejects an empty reservationID before this is called.
func (s *ListingService) ReleaseStock(ctx context.Context, reservationID string) error {
	outcome, err := s.repo.ReleaseReservation(ctx, reservationID)
	if err != nil {
		return err
	}
	switch outcome {
	case store.ReleaseUnknown:
		slog.WarnContext(ctx, "release of unknown reservation_id treated as no-op", "reservation_id", reservationID)
	case store.ReleaseNoOp:
		slog.WarnContext(ctx, "release of already released reservation treated as no-op", "reservation_id", reservationID)
	}
	return nil
}

// ReserveStockIdempotent reserves stock keyed on a stable reservationID so a
// retried checkout decrements exactly once. The reservation is held for the
// configured reservation TTL, after which SweepExpiredReservations restores it
// unless it was committed. Re-reserving the id of a released reservation fails
// with store.ErrReservationReleased. reservationID is required
// (store.ErrReservationIDRequired): there is no ledger-less decrement.
func (s *ListingService) ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32) error {
	return s.repo.ReserveStockIdempotent(ctx, reservationID, listingID, variantID, quantity, time.Now().Add(s.ReservationTTL()))
}

// SweepExpiredReservations releases reservations past their TTL and restores the
// reserved stock (AD3 domain side), returning how many were released. It is
// idempotent and safe to run on a periodic schedule (see ReservationSweeper).
func (s *ListingService) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	return s.repo.SweepExpiredReservations(ctx, now)
}

// CommitReservation makes an active reservation permanent so the TTL sweeper
// never restores it. Idempotent on reservationID. Errors are the repository
// sentinels ErrReservationReleased / ErrReservationNotFound; the handler maps
// them to FAILED_PRECONDITION / NOT_FOUND.
func (s *ListingService) CommitReservation(ctx context.Context, reservationID string) error {
	return s.repo.CommitReservation(ctx, reservationID)
}

func isSellerStatus(s string) bool { return s == "draft" || s == "published" }

// createStatus lets a seller create only DRAFT or PUBLISHED; anything else is DRAFT.
func createStatus(requested string) string {
	if r := strings.ToLower(requested); isSellerStatus(r) {
		return r
	}
	return "draft"
}

func isLockedStatus(st string) bool {
	st = strings.ToLower(st)
	return st == "banned" || st == "held"
}

// resolveSellerUpdate decides the status of a seller write (the server owns
// status): locked listings are refused, rejected ones go to manual review, and
// a draft publish or content edit of a published listing is screened. It also
// carries the stored moderation fields over and pins the loaded version so a
// racing moderation decision is detected.
func (s *ListingService) resolveSellerUpdate(ctx context.Context, existing store.Listing, l *store.Listing) error {
	stored := strings.ToLower(existing.Status)
	if isLockedStatus(stored) {
		return ErrStatusLocked
	}
	requested := strings.ToLower(l.Status)
	l.ModerationReasonCode = existing.ModerationReasonCode
	l.ModerationNote = existing.ModerationNote
	l.ModerationDecidedAt = existing.ModerationDecidedAt
	l.ScreenFlags = existing.ScreenFlags
	l.SubmittedAt = existing.SubmittedAt
	l.LoadedStatus = existing.Status
	l.LoadedUpdatedAt = existing.UpdatedAt

	switch {
	case stored == "rejected":
		s.applyScreen(ctx, l)
		l.Status = "pending_review"
		if l.SubmittedAt.IsZero() || existing.SubmittedAt.Equal(l.SubmittedAt) {
			l.SubmittedAt = time.Now()
		}
	case isSellerStatus(stored):
		eff := stored
		if isSellerStatus(requested) {
			eff = requested
		}
		l.Status = eff
		contentChanged := l.Title != existing.Title || l.Description != existing.Description || l.CategoryID != existing.CategoryID
		if eff == "published" && (stored == "draft" || contentChanged) {
			s.applyScreen(ctx, l)
		}
	default:
		// pending_review and other server-only statuses are sticky.
		l.Status = existing.Status
	}
	return nil
}

// applyScreen screens l and sets Status (published or pending_review),
// ScreenFlags and SubmittedAt. A keyword hit skips the AI; a missing screener,
// an AI error or any non-ALLOW verdict goes to pending_review.
func (s *ListingService) applyScreen(ctx context.Context, l *store.Listing) {
	status, flags := s.screen(ctx, *l)
	l.Status = status
	l.ScreenFlags = flags
	if status == "pending_review" {
		l.SubmittedAt = time.Now()
	}
}

func (s *ListingService) screen(ctx context.Context, l store.Listing) (string, []string) {
	if s.kw != nil {
		if hits := s.kw.Match(l.Title + "\n" + l.Description); len(hits) > 0 {
			flags := make([]string, 0, len(hits))
			for _, h := range hits {
				flags = append(flags, "keyword:"+h)
			}
			return "pending_review", flags
		}
	}
	if s.ai == nil {
		return "pending_review", nil
	}
	res, err := s.ai.ScreenListing(ctx, screen.ScreenInput{ListingID: l.ID, Title: l.Title, Description: l.Description, CategoryID: l.CategoryID})
	if err != nil {
		slog.WarnContext(ctx, "listing screen failed, routing to review", slog.String("listing_id", l.ID), slog.Any("err", err))
		return "pending_review", nil
	}
	if res.GetVerdict() == aiv1.ScreenVerdict_SCREEN_VERDICT_ALLOW {
		return "published", []string{}
	}
	var flags []string
	for _, lb := range res.GetLabels() {
		flags = append(flags, "ai:"+lb)
	}
	return "pending_review", flags
}
