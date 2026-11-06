package referral

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-referral/internal/store"
)

const processedEventRetention = 7 * 24 * time.Hour

type Sweeper struct {
	repo   store.ReferralRepository
	cfg    Config
	now    func() time.Time
	logger *slog.Logger
}

func NewSweeper(repo store.ReferralRepository, opts ...Option) *Sweeper {
	s := newSettings(opts)
	return &Sweeper{repo: repo, cfg: s.cfg, now: s.now, logger: s.logger}
}

func (s *Sweeper) Sweep(ctx context.Context) (int, error) {
	now := s.now()
	released, releaseErr := s.repo.ReleaseDue(ctx, store.ReleaseInput{
		At:          now,
		DailyCap:    s.cfg.DailyCap,
		LifetimeCap: s.cfg.LifetimeCap,
		Location:    s.cfg.CapLocation,
	})
	_, purgeErr := s.repo.PurgeProcessedEvents(ctx, now.Add(-processedEventRetention))
	return released, errors.Join(releaseErr, purgeErr)
}

func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		released, err := s.Sweep(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			s.logger.ErrorContext(ctx, "referral sweep failed", "released", released, "err", err)
		case released > 0:
			s.logger.InfoContext(ctx, "referral rewards released", "released", released)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
