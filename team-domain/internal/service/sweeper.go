package service

import (
	"context"
	"log/slog"
	"time"
)

// ReservationSweeper periodically releases stock held by reservations past their
// TTL (AD3 domain side). It mirrors the outbox relayer's start/stop shape: Run
// polls until ctx is cancelled and is meant to run in its own goroutine, while
// SweepOnce is exported so tests (and a manual trigger) can drive a single
// deterministic pass. The sweep itself is idempotent, so an occasional missed or
// doubled tick is harmless.
type ReservationSweeper struct {
	svc      *ListingService
	interval time.Duration
	logger   *slog.Logger
}

// NewReservationSweeper builds a sweeper over the service. A non-positive
// interval defaults to one minute; a nil logger uses slog.Default().
func NewReservationSweeper(svc *ListingService, interval time.Duration, logger *slog.Logger) *ReservationSweeper {
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReservationSweeper{svc: svc, interval: interval, logger: logger}
}

// SweepOnce runs a single sweep as of now, returning the number of reservations
// released.
func (s *ReservationSweeper) SweepOnce(ctx context.Context, now time.Time) (int, error) {
	return s.svc.SweepExpiredReservations(ctx, now)
}

// Run sweeps on the configured interval until ctx is cancelled. A sweep error is
// transient (e.g. a DB blip) — it is logged and retried on the next tick.
func (s *ReservationSweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			released, err := s.SweepOnce(ctx, now)
			if err != nil {
				s.logger.WarnContext(ctx, "reservation sweep failed", slog.Any("err", err))
				continue
			}
			if released > 0 {
				s.logger.InfoContext(ctx, "released expired reservations", slog.Int("released", released))
			}
		}
	}
}
