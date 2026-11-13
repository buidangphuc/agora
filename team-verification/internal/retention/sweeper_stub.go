package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-verification/internal/objects"
)

type Config struct {
	Retention time.Duration
	Interval  time.Duration
	BatchSize int
}

type SweepResult struct{ Documents int }

type Sweeper struct{}

func NewSweeper(_ *pgxpool.Pool, _ objects.Store, _ Config, _ *slog.Logger) *Sweeper {
	return &Sweeper{}
}

func (s *Sweeper) SweepOnce(_ context.Context, _ time.Time) (SweepResult, error) {
	return SweepResult{}, nil
}
