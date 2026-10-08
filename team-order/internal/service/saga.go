package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// ErrReservationLost: a reservation can no longer back an order — team-domain
// refused the commit because it was already released (TTL sweep) or is unknown, or
// placement found it no longer RESERVED. No order is placed. -> FAILED_PRECONDITION.
var ErrReservationLost = repository.ErrReservationLost

// ErrPlacementUnknown: placement failed and it could not be established whether
// the orders exist (only some do, or the lookup failed). Nothing is released. ->
// INTERNAL.
var ErrPlacementUnknown = errors.New("order placement outcome unknown")

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

// WithOrderPlacer injects the OrderPlacer (tests wrap the real one with faults).
func WithOrderPlacer(p repository.OrderPlacer) OrderServiceOption {
	return func(s *OrderService) {
		if p != nil {
			s.placer = p
		}
	}
}

// defaultPlacer picks the atomic placer of the wired stores: the Postgres order
// repository places in one transaction; the in-memory stores place under their
// locks. Any other order store (a test double) gets the non-atomic sequential
// fallback.
func defaultPlacer(orders repository.OrderRepository, sagas repository.SagaRepository) repository.OrderPlacer {
	if p, ok := orders.(repository.OrderPlacer); ok {
		return p
	}
	mo, okO := orders.(*repository.InMemoryOrderRepository)
	ms, okS := sagas.(*repository.InMemorySagaRepository)
	if okO && okS {
		return repository.NewInMemoryOrderPlacer(mo, ms)
	}
	return sequentialPlacer{orders: orders, sagas: sagas}
}

// sequentialPlacer is NOT atomic: CreateOrder per order, then bind and complete.
// It exists only so order-store test doubles keep working.
type sequentialPlacer struct {
	orders repository.OrderRepository
	sagas  repository.SagaRepository
}

func (p sequentialPlacer) PlaceOrders(ctx context.Context, sagaID string, placed []repository.PlacedOrder) ([]repository.Order, error) {
	out := make([]repository.Order, 0, len(placed))
	for _, po := range placed {
		saved, err := p.orders.CreateOrder(ctx, po.Order)
		if err != nil {
			return nil, err
		}
		for _, id := range po.ReservationIDs {
			if err := p.sagas.CommitReservation(ctx, id, saved.ID); err != nil {
				return nil, err
			}
		}
		out = append(out, saved)
	}
	if err := p.sagas.UpdateSagaStatus(ctx, sagaID, repository.SagaStatusCompleted); err != nil {
		return nil, err
	}
	return out, nil
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

// ReservationID is the reservation id of one cart item within one checkout
// ATTEMPT (its saga). Every attempt has its own saga id, so two attempts never
// share a reservation: a checkout that follows a compensated one reserves normally
// instead of hitting team-domain's refusal to re-reserve a released id. The same
// attempt and item always map to the same id. The idempotency key is deliberately
// not part of the seed: a keyed replay never reserves (it returns the first
// attempt's orders) and concurrent same-key requests collapse on the saga's unique
// index before reserving (design D7).
func ReservationID(sagaID string, item repository.CartItem) string {
	seed := strings.Join([]string{
		"attempt", sagaID, item.ID, item.ListingID, item.VariantID, fmt.Sprintf("q%d", item.Quantity),
	}, "|")
	return uuid.NewSHA1(reservationNamespace, []byte(seed)).String()
}

// reserveStock asks team-domain to hold one reservation's stock and records the
// outcome durably:
//
//	success                       -> RESERVED
//	Success=false (short stock)   -> FAILED (nothing held), ErrInsufficientStock
//	transport error (no answer)   -> RELEASE_FAILED: team-domain may have applied
//	                                 it, so compensation releases it by id (a no-op
//	                                 when nothing was held), ErrInsufficientStock
//	any other error               -> FAILED, ErrInsufficientStock
func (s *OrderService) reserveStock(ctx context.Context, res repository.Reservation, title string) error {
	resp, rerr := s.domainClient.ReserveStock(ctx, &listingv1.ReserveStockRequest{
		ListingId:     res.ListingID,
		VariantId:     res.VariantID,
		Quantity:      res.Quantity,
		ReservationId: res.ID,
	})
	if rerr == nil && resp.GetSuccess() {
		if uerr := s.sagaRepo.UpdateReservationStatus(ctx, res.ID, repository.ReservationStatusReserved); uerr != nil {
			// Not fatal: placement requires RESERVED, so a lost write fails the
			// checkout definitively and compensation releases by id.
			s.logger.WarnContext(ctx, "failed to mark reservation reserved",
				slog.String("reservation_id", res.ID), slog.Any("err", uerr))
		}
		return nil
	}
	if rerr == nil {
		rerr = fmt.Errorf("reserve stock declined: %s", resp.GetMessage())
	}
	next := repository.ReservationStatusFailed
	if isTransportError(rerr) {
		next = repository.ReservationStatusReleaseFailed
	}
	s.logger.WarnContext(ctx, "stock reservation failed",
		slog.String("listing_id", res.ListingID),
		slog.String("variant_id", res.VariantID),
		slog.Int("qty", int(res.Quantity)),
		slog.Any("err", rerr),
	)
	// Recorded on a fresh context: the request context may be what failed.
	bg, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	if uerr := s.sagaRepo.UpdateReservationStatus(bg, res.ID, next); uerr != nil {
		s.logger.WarnContext(ctx, "failed to record reservation outcome",
			slog.String("reservation_id", res.ID), slog.Any("err", uerr))
	}
	return fmt.Errorf("%w: %s (%v)", ErrInsufficientStock, title, rerr)
}

// isTransportError reports whether err leaves the remote outcome unknown.
func isTransportError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return true
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Unknown:
		return true
	}
	return false
}

// commitDomainReservations commits every reservation in team-domain
// (CommitReservation, idempotent). FAILED_PRECONDITION / NOT_FOUND mean the hold is
// gone (swept or unknown): ErrReservationLost, no order is placed. Any other error,
// UNIMPLEMENTED included, fails the checkout too: the commit is not confirmed, so
// placing the order could let team-domain's sweep give its stock away.
func (s *OrderService) commitDomainReservations(ctx context.Context, reservations []repository.Reservation) error {
	for _, res := range reservations {
		_, err := s.domainClient.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: res.ID})
		if err == nil {
			continue
		}
		switch status.Code(err) {
		case codes.FailedPrecondition, codes.NotFound:
			s.logger.WarnContext(ctx, "reservation no longer held at commit; refusing to place the order",
				slog.String("reservation_id", res.ID), slog.String("listing_id", res.ListingID), slog.Any("err", err))
			return fmt.Errorf("%w: listing %s", ErrReservationLost, res.ListingID)
		default:
			return fmt.Errorf("commit reservation %s: %w", res.ID, err)
		}
	}
	return nil
}

// failAndCompensate undoes a checkout attempt that placed no order: it releases
// every reservation of the saga that holds (or may hold) stock, by its id, the
// voucher hold, and marks the saga COMPENSATED. It reads the durable reservation
// set and runs on a fresh background context (AD3).
func (s *OrderService) failAndCompensate(sagaID, voucherReservationID string) {
	bg, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	all, err := s.sagaRepo.ListReservationsBySaga(bg, sagaID)
	if err != nil {
		s.logger.ErrorContext(bg, "failed to load reservations for compensation; the sweep will release them",
			slog.String("saga_id", sagaID), slog.Any("err", err))
	}
	s.compensate(all)
	s.releaseVoucher(bg, voucherReservationID)
	if uerr := s.sagaRepo.UpdateSagaStatus(bg, sagaID, repository.SagaStatusCompensated); uerr != nil {
		s.logger.ErrorContext(bg, "failed to mark saga compensated",
			slog.String("saga_id", sagaID), slog.Any("err", uerr))
	}
}

// placementState is what reconcilePlacement found after a PlaceOrders error.
type placementState int

const (
	placementUnknown  placementState = iota // partial, or the lookup failed
	placementAbsent                         // none of the orders exist
	placementComplete                       // every order exists
)

// reconcilePlacement looks every pre-generated order id up on a fresh context.
func (s *OrderService) reconcilePlacement(placed []repository.PlacedOrder) ([]repository.Order, placementState) {
	ctx, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	var found []repository.Order
	for _, p := range placed {
		o, err := s.orderRepo.GetOrder(ctx, p.Order.ID)
		switch {
		case err == nil:
			found = append(found, o)
		case errors.Is(err, repository.ErrOrderNotFound):
		default:
			s.logger.ErrorContext(ctx, "cannot reconcile order placement: lookup failed; not compensating",
				slog.String("order_id", p.Order.ID), slog.Any("err", err))
			return nil, placementUnknown
		}
	}
	switch len(found) {
	case 0:
		return nil, placementAbsent
	case len(placed):
		return found, placementComplete
	}
	s.logger.ErrorContext(ctx, "order placement is PARTIAL; not compensating",
		slog.Int("placed", len(placed)), slog.Int("found", len(found)))
	return nil, placementUnknown
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
