package main

// TEST-STUB: compile-only placeholder; the implementer deletes this file and
// defines runLifecycleSweeper (and lifecycleSweeper) in main.go.

import (
	"context"
	"log/slog"
	"time"
)

type lifecycleSweeper interface {
	SweepLifecycle(ctx context.Context, now time.Time)
}

func runLifecycleSweeper(ctx context.Context, svc lifecycleSweeper, interval time.Duration, logger *slog.Logger) {
}
