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
	defaultReservationTTL = 15 * time.Minute
	defaultReleaseTimeout = 30 * time.Second
	// defaultInlineReleaseTimeout bounds the release a buyer-facing cancel waits
	// for; it must stay under the gateway's per-call deadline (5s) so a slow or
	// stopped team-domain parks the release for the sweep instead of failing
	// the cancel at the edge.
	defaultInlineReleaseTimeout = 2 * time.Second
	defaultReleaseMaxAttempts   = 3
	defaultReleaseBackoff       = 100 * time.Millisecond
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
	timeout       time.Duration
	inlineTimeout time.Duration
	maxAttempts   int
	backoff       time.Duration
}

func (c releaseRetryConfig) withDefaults() releaseRetryConfig {
	if c.timeout <= 0 {
		c.timeout = defaultReleaseTimeout
	}
	if c.inlineTimeout <= 0 {
		c.inlineTimeout = defaultInlineReleaseTimeout
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

// compensate releases the stock held by reservations no order owns (RESERVED, or
// parked RELEASE_FAILED) on a fresh background context with its own deadline
// (AD3). A release that keeps failing is parked RELEASE_FAILED for the sweep.
func (s *OrderService) compensate(reservations []repository.Reservation) {
	s.releaseAll(reservations, repository.ReservationStatusReserved, repository.ReservationStatusReleaseFailed)
}

// releaseHeld releases reservations of Cancelled orders (COMMITTED, or parked
// RELEASE_FAILED) by their original id.
func (s *OrderService) releaseHeld(reservations []repository.Reservation) {
	s.releaseAll(reservations, repository.ReservationStatusCommitted, repository.ReservationStatusReleaseFailed)
}

func (s *OrderService) releaseAll(reservations []repository.Reservation, statuses ...repository.ReservationStatus) {
	if len(reservations) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	for _, res := range reservations {
		eligible := false
		for _, st := range statuses {
			if res.Status == st {
				eligible = true
			}
		}
		if eligible {
			_, _ = s.releaseOrPark(ctx, res)
		}
	}
}

// releaseOrPark releases one reservation by its original id and marks it
// RELEASED, or parks it RELEASE_FAILED when the release keeps failing. released
// reports whether the stock is back; err is non-nil only when a failed release
// could not even be parked.
func (s *OrderService) releaseOrPark(ctx context.Context, res repository.Reservation) (released bool, err error) {
	if rerr := s.releaseReservationWithRetry(ctx, res); rerr != nil {
		s.logger.ErrorContext(ctx, "release failed; parking for the sweep",
			slog.String("reservation_id", res.ID),
			slog.String("listing_id", res.ListingID),
			slog.Any("err", rerr),
		)
		if uerr := s.sagaRepo.UpdateReservationStatus(ctx, res.ID, repository.ReservationStatusReleaseFailed); uerr != nil {
			s.logger.ErrorContext(ctx, "failed to park reservation as release-failed",
				slog.String("reservation_id", res.ID), slog.Any("err", uerr))
			return false, fmt.Errorf("park reservation %s: %w", res.ID, uerr)
		}
		return false, nil
	}
	if uerr := s.sagaRepo.UpdateReservationStatus(ctx, res.ID, repository.ReservationStatusReleased); uerr != nil {
		// The stock is back; re-releasing a stale row is a no-op in team-domain.
		s.logger.ErrorContext(ctx, "failed to mark reservation released",
			slog.String("reservation_id", res.ID), slog.Any("err", uerr))
	}
	return true, nil
}

// releaseReservationWithRetry calls ReleaseStock up to maxAttempts with backoff,
// keyed by the reservation's original id (team-domain restores the stored
// quantity once; the quantity sent is ignored).
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

// SweepExpiredReservations is the team-order sweep. Each tick it:
//
//  1. releases reservations past their TTL that no order owns (RESERVED, or parked
//     RELEASE_FAILED) — a checkout that crashed or failed to compensate;
//  2. releases reservations still held by Cancelled orders (COMMITTED, or parked
//     RELEASE_FAILED) — a cancel that crashed between its claim and its release, or
//     whose release failed. Only orders cancelled at least one release timeout ago,
//     so the sweep never races a cancel still releasing;
//  3. settles checkout attempts still PENDING a full TTL after they started.
//
// A COMPLETED attempt's reservations (COMMITTED, live order) are never touched.
// Release is idempotent on reservation_id, so the sweep is safe to repeat. It
// returns the number of reservations released.
func (s *OrderService) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	stale, err := s.sagaRepo.FindReleasable(ctx, now, 100)
	if err != nil {
		return 0, fmt.Errorf("find releasable reservations: %w", err)
	}
	cancelled, err := s.sagaRepo.FindHeldByCancelledOrders(ctx, now.Add(-s.releaseCfg.timeout), 100)
	if err != nil {
		return 0, fmt.Errorf("find reservations of cancelled orders: %w", err)
	}
	// A parked reservation of a cancelled order can be in both sets.
	seen := make(map[string]bool, len(stale))
	for _, r := range stale {
		seen[r.ID] = true
	}
	var heldOnly []repository.Reservation
	for _, r := range cancelled {
		if !seen[r.ID] {
			heldOnly = append(heldOnly, r)
		}
	}
	all := append(append([]repository.Reservation{}, stale...), heldOnly...)
	before := s.countReleased(ctx, all)
	s.compensate(stale)
	s.releaseHeld(heldOnly)
	released := s.countReleased(ctx, all) - before
	s.settleStalePendingSagas(ctx, now)
	return released, nil
}

// settleStalePendingSagas compensates checkout attempts still PENDING a full
// reservation TTL after they started (a crash): their held reservations are
// released, then the saga is marked COMPENSATED, which frees its idempotency key.
// A saga that still holds stock after the release is left for the next tick. A
// saga whose reservations are bound to an order (only possible with a non-atomic
// placer) is marked COMPLETED instead.
func (s *OrderService) settleStalePendingSagas(ctx context.Context, now time.Time) {
	sagas, err := s.sagaRepo.FindStalePendingSagas(ctx, now.Add(-s.reservationTTL), 100)
	if err != nil {
		s.logger.ErrorContext(ctx, "find stale pending sagas", slog.Any("err", err))
		return
	}
	for _, sg := range sagas {
		reservations, err := s.sagaRepo.ListReservationsBySaga(ctx, sg.ID)
		if err != nil {
			s.logger.ErrorContext(ctx, "load reservations of a stale saga", slog.String("saga_id", sg.ID), slog.Any("err", err))
			continue
		}
		placed := false
		var holding []repository.Reservation
		for _, res := range reservations {
			switch {
			case res.OrderID != "":
				placed = true
			case res.Status == repository.ReservationStatusReserved || res.Status == repository.ReservationStatusReleaseFailed:
				holding = append(holding, res)
			}
		}
		next := repository.SagaStatusCompensated
		if placed {
			next = repository.SagaStatusCompleted
		} else {
			s.compensate(holding)
			settled := true
			for _, res := range holding {
				if got, gerr := s.sagaRepo.GetReservation(ctx, res.ID); gerr != nil || got.Status != repository.ReservationStatusReleased {
					settled = false
				}
			}
			if !settled {
				continue
			}
		}
		if err := s.sagaRepo.UpdateSagaStatus(ctx, sg.ID, next); err != nil {
			s.logger.ErrorContext(ctx, "failed to settle a stale saga", slog.String("saga_id", sg.ID), slog.Any("err", err))
			continue
		}
		s.logger.WarnContext(ctx, "settled a stale pending checkout attempt",
			slog.String("saga_id", sg.ID), slog.Int("status", int(next)))
	}
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
