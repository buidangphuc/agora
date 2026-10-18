package service

// TEST-STUB: compile-only placeholder written by test-author for task 3.
// The implementer deletes this file and creates session_sweeper.go.

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-identity/internal/repository"
)

type SessionSweeperConfig struct {
	SessionRetention      time.Duration
	LoginHistoryRetention time.Duration
	Interval              time.Duration
	BatchSize             int
}

type SweepResult struct {
	Sessions    int
	LoginEvents int
}

type SessionSweeper struct{}

func NewSessionSweeper(_ repository.SessionRepository, _ SessionSweeperConfig, _ *slog.Logger) *SessionSweeper {
	return &SessionSweeper{}
}

func (s *SessionSweeper) SweepOnce(_ context.Context, _ time.Time) (SweepResult, error) {
	return SweepResult{}, nil
}

func (s *SessionSweeper) Run(_ context.Context) {}
