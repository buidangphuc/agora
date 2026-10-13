package listing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
)

// ReservationSweeper periodically releases stock held by reservations past their
// TTL (AD3 domain side). It is a lifecycle.Component: Run polls until ctx is
// cancelled and returns nil, while SweepOnce is exported so tests (and a manual trigger) can drive a single
// deterministic pass. The sweep itself is idempotent, so an occasional missed or
// doubled tick is harmless.
type ReservationSweeper struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

// NewReservationSweeper builds a sweeper over the service. A non-positive
// interval defaults to one minute; a nil logger uses slog.Default().
func NewReservationSweeper(svc *Service, interval time.Duration, logger *slog.Logger) *ReservationSweeper {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReservationSweeper{svc: svc, interval: interval, logger: logger}
}

// Interval returns the effective polling interval.
func (s *ReservationSweeper) Interval() time.Duration { return s.interval }

// SweepOnce runs a single sweep as of now, returning the number of reservations
// released.
func (s *ReservationSweeper) SweepOnce(ctx context.Context, now time.Time) (int, error) {
	return s.svc.SweepExpiredReservations(ctx, now)
}

// Run sweeps on the configured interval until ctx is cancelled. A sweep error is
// transient (e.g. a DB blip): lifecycle.Every logs it and the next tick retries.
func (s *ReservationSweeper) Run(ctx context.Context) error {
	// The effective values are logged as strings ("15m0s") so operators and the
	// e2e config scenarios can read them in either log format.
	s.logger.InfoContext(ctx, "reservation sweeper started",
		"reservation_ttl", s.svc.ReservationTTL().String(),
		"sweep_interval", s.interval.String())
	every, err := lifecycle.Every(s.interval, func(ctx context.Context) error {
		released, err := s.SweepOnce(ctx, time.Now())
		if err != nil {
			return fmt.Errorf("reservation sweep failed: %w", err)
		}
		if released > 0 {
			s.logger.InfoContext(ctx, "released expired reservations", "released", released)
		}
		return nil
	}, lifecycle.WithLogger(s.logger))
	if err != nil {
		return err
	}
	return every.Run(ctx)
}
