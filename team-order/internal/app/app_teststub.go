package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"

	"github.com/buidangphuc/team-order/internal/config"
)

var wrapComponent = func(_ string, c lifecycle.Component) lifecycle.Component { return c }

type reservationSweeper interface {
	SweepExpiredReservations(ctx context.Context, now time.Time) (int, error)
}

type lifecycleSweeper interface {
	SweepLifecycle(ctx context.Context, now time.Time)
}

func Run(_ context.Context, _ config.Settings) error { return errors.New("app.Run: not implemented") }

func reservationSweep(_ reservationSweeper, _ time.Duration, _ *slog.Logger) (lifecycle.Component, error) {
	return lifecycle.Func(func(ctx context.Context) error { <-ctx.Done(); return nil }), nil
}

func lifecycleSweep(_ lifecycleSweeper, _ time.Duration, _ *slog.Logger) (lifecycle.Component, error) {
	return lifecycle.Func(func(ctx context.Context) error { <-ctx.Done(); return nil }), nil
}
