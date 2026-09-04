package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

// Saga tuning defaults. Compensation runs on a fresh background context with its
// own deadline (AD3) so a cancelled/expired request context can never abort the
// release of already-reserved stock.
const (
	defaultReservationTTL     = 15 * time.Minute
	defaultReleaseTimeout     = 30 * time.Second
	defaultReleaseMaxAttempts = 3
	defaultReleaseBackoff     = 100 * time.Millisecond
)

// reservationNamespace is a fixed UUID namespace so ReservationID is deterministic
// across processes and restarts.
var reservationNamespace = uuid.MustParse("6ba7b814-9dad-11d1-80b4-00c04fd430c8")

// releaseRetryConfig bounds a compensation release's retries.
type releaseRetryConfig struct {
	timeout     time.Duration
	maxAttempts int
	backoff     time.Duration
}

func (c releaseRetryConfig) withDefaults() releaseRetryConfig {
	if c.timeout <= 0 {
		c.timeout = defaultReleaseTimeout
	}
	if c.maxAttempts <= 0 {
		c.maxAttempts = defaultReleaseMaxAttempts
	}
	if c.backoff <= 0 {
		c.backoff = defaultReleaseBackoff
	}
	return c
}

// OrderServiceOption customises an OrderService without breaking the positional
// constructor signature (so cmd/server/main.go keeps compiling).
type OrderServiceOption func(*OrderService)

// WithSagaRepository wires the durable saga/reservation store (AD3). When unset,
// the service falls back to an in-memory store.
func WithSagaRepository(repo repository.SagaRepository) OrderServiceOption {
	return func(s *OrderService) {
		if repo != nil {
			s.sagaRepo = repo
		}
	}
}

// WithReservationTTL overrides the reservation time-to-live before the sweep
// reclaims it.
func WithReservationTTL(ttl time.Duration) OrderServiceOption {
	return func(s *OrderService) {
		if ttl > 0 {
			s.reservationTTL = ttl
		}
	}
}

// WithReleaseRetry overrides compensation-release retry bounds.
func WithReleaseRetry(timeout time.Duration, maxAttempts int, backoff time.Duration) OrderServiceOption {
	return func(s *OrderService) {
		s.releaseCfg = releaseRetryConfig{timeout: timeout, maxAttempts: maxAttempts, backoff: backoff}.withDefaults()
	}
}

// ReservationID returns a stable reservation id per (buyer, cart-item, attempt)
// (AD5/M6). It is deterministic in the cart item's identity, so a retried checkout
// of the same cart item reuses the same id and team-domain's idempotent reserve
// decrements stock only once. A cart item is removed only after its order commits,
// so its id is the natural per-attempt key.
func ReservationID(buyerID string, item repository.CartItem) string {
	seed := strings.Join([]string{
		buyerID, item.ID, item.ListingID, item.VariantID, fmt.Sprintf("q%d", item.Quantity),
	}, "|")
	return uuid.NewSHA1(reservationNamespace, []byte(seed)).String()
}

// compensate releases the stock held by un-committed reservations. It ALWAYS runs
// on a fresh context.Background() with its own deadline (AD3): the request context
// may already be cancelled or timed out when compensation is triggered, and the
// stock must still be returned. COMMITTED reservations are skipped so a persisted
// order's stock is never released (M7). A release that keeps failing is parked as
// RELEASE_FAILED for the TTL sweep to retry — never silently discarded.
func (s *OrderService) compensate(reservations []repository.Reservation) {
	if len(reservations) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()

	for _, res := range reservations {
		// Only reservations that actually hold stock and are not committed.
		if res.Status != repository.ReservationStatusReserved && res.Status != repository.ReservationStatusReleaseFailed {
			continue
		}
		if err := s.releaseReservationWithRetry(ctx, res); err != nil {
			s.logger.ErrorContext(ctx, "compensation release failed; parking for sweep",
				slog.String("reservation_id", res.ID),
				slog.String("listing_id", res.ListingID),
				slog.Any("err", err),
			)
			if uerr := s.sagaRepo.UpdateReservationStatus(ctx, res.ID, repository.ReservationStatusReleaseFailed); uerr != nil {
				s.logger.ErrorContext(ctx, "failed to park reservation as release-failed",
					slog.String("reservation_id", res.ID), slog.Any("err", uerr))
			}
			continue
		}
		if uerr := s.sagaRepo.UpdateReservationStatus(ctx, res.ID, repository.ReservationStatusReleased); uerr != nil {
			s.logger.ErrorContext(ctx, "failed to mark reservation released",
				slog.String("reservation_id", res.ID), slog.Any("err", uerr))
		}
	}
}

// releaseReservationWithRetry calls ReleaseStock up to maxAttempts with backoff.
// It passes the stable reservation_id so team-domain can make the release
// idempotent against the matching reserve.
func (s *OrderService) releaseReservationWithRetry(ctx context.Context, res repository.Reservation) error {
	var lastErr error
	for attempt := 1; attempt <= s.releaseCfg.maxAttempts; attempt++ {
		_, err := s.domainClient.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{
			ListingId:     res.ListingID,
			VariantId:     res.VariantID,
			Quantity:      res.Quantity,
			ReservationId: res.ID,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < s.releaseCfg.maxAttempts {
			select {
			case <-ctx.Done():
				return fmt.Errorf("release cancelled: %w", ctx.Err())
			case <-time.After(s.releaseCfg.backoff * time.Duration(attempt)):
			}
		}
	}
	return fmt.Errorf("release stock after %d attempts: %w", s.releaseCfg.maxAttempts, lastErr)
}

// SweepExpiredReservations reclaims stock held by reservations whose TTL has
// elapsed and that were never committed to an order — the recovery path for a
// crash/timeout between ReserveStock and order persistence (SA-C2). It runs on a
// background context and returns the number of reservations released. It is safe
// to call repeatedly (a released reservation is no longer releasable).
func (s *OrderService) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	stale, err := s.sagaRepo.FindReleasable(ctx, now, 100)
	if err != nil {
		return 0, fmt.Errorf("find releasable reservations: %w", err)
	}
	if len(stale) == 0 {
		return 0, nil
	}
	before := s.countReleased(ctx, stale)
	s.compensate(stale)
	after := s.countReleased(ctx, stale)
	return after - before, nil
}

func (s *OrderService) countReleased(ctx context.Context, reservations []repository.Reservation) int {
	n := 0
	for _, r := range reservations {
		got, err := s.sagaRepo.GetReservation(ctx, r.ID)
		if err == nil && got.Status == repository.ReservationStatusReleased {
			n++
		}
	}
	return n
}
