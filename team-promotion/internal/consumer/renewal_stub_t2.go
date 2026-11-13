package consumer

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-promotion/internal/subscription"
)

type RenewalConfig struct {
	RetryWindow time.Duration
	Now         func() time.Time
	Logger      *slog.Logger
}

type RenewalSweeper struct{}

func NewRenewalSweeper(_ subscription.Repository, _ subscription.PaymentCharger, _ RenewalConfig) *RenewalSweeper {
	return &RenewalSweeper{}
}

func (s *RenewalSweeper) Sweep(_ context.Context) (int, error) { return 0, nil }
