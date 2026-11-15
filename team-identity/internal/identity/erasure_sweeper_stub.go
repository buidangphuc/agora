package identity

// RED-FIRST STUB for task t3-eraser-sweeper-signin: the implementer replaces this whole file
// with the real erasure sweeper (and deletes this marker).

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

type ErasureRepository interface {
	EraseDue(ctx context.Context, now time.Time, limit int, enqueue store.ErasedEnqueueFn) (store.ErasureResult, error)
}

type ErasureSweeperConfig struct {
	Interval  time.Duration
	BatchSize int
}

type ErasureSweeper struct{}

func NewErasureSweeper(ErasureRepository, ErasureSweeperConfig, *slog.Logger) *ErasureSweeper {
	return &ErasureSweeper{}
}

func (s *ErasureSweeper) WithClock(func() time.Time) *ErasureSweeper { return s }

func (s *ErasureSweeper) SweepOnce(context.Context, time.Time) (store.ErasureResult, error) {
	return store.ErasureResult{}, nil
}

func (s *ErasureSweeper) Tick(context.Context) error { return nil }
