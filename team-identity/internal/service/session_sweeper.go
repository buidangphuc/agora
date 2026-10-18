package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-identity/internal/repository"
)

const defaultSweepBatchSize = 500

// SessionSweeperConfig controls retention windows and the sweep cadence.
type SessionSweeperConfig struct {
	SessionRetention      time.Duration
	LoginHistoryRetention time.Duration
	Interval              time.Duration
	BatchSize             int
}

// SweepResult holds the number of rows purged by one sweep.
type SweepResult struct {
	Sessions    int
	LoginEvents int
}

// SessionSweeper periodically purges ended sessions and old login history.
type SessionSweeper struct {
	repo   repository.SessionRepository
	cfg    SessionSweeperConfig
	logger *slog.Logger
}

func NewSessionSweeper(repo repository.SessionRepository, cfg SessionSweeperConfig, logger *slog.Logger) *SessionSweeper {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultSweepBatchSize
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionSweeper{repo: repo, cfg: cfg, logger: logger}
}

// SweepOnce purges in batches until a short batch is returned. On error it returns the
// counts purged so far together with the error.
func (s *SessionSweeper) SweepOnce(ctx context.Context, now time.Time) (SweepResult, error) {
	var res SweepResult
	for {
		n, err := s.repo.PurgeSessions(ctx, now, s.cfg.SessionRetention, s.cfg.BatchSize)
		res.Sessions += n
		if err != nil {
			return res, err
		}
		if n < s.cfg.BatchSize {
			break
		}
	}
	cutoff := now.Add(-s.cfg.LoginHistoryRetention)
	for {
		n, err := s.repo.PurgeLoginHistory(ctx, cutoff, s.cfg.BatchSize)
		res.LoginEvents += n
		if err != nil {
			return res, err
		}
		if n < s.cfg.BatchSize {
			break
		}
	}
	return res, nil
}

// Run sweeps immediately and then every Interval until ctx is cancelled. Errors are logged
// as warnings and retried on the next tick. Logs carry counts only.
func (s *SessionSweeper) Run(ctx context.Context) {
	interval := s.cfg.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		res, err := s.SweepOnce(ctx, time.Now())
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			s.logger.Warn("session retention sweep failed; will retry",
				"sessions_purged", res.Sessions, "login_events_purged", res.LoginEvents)
		case res.Sessions > 0 || res.LoginEvents > 0:
			s.logger.Info("session retention sweep done",
				"sessions_purged", res.Sessions, "login_events_purged", res.LoginEvents)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
