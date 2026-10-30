package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s, err := config.Load()
	if err != nil {
		slog.Error("load settings", "err", err)
		return 1
	}
	if err := app.RunGA4Relay(ctx, s); err != nil {
		slog.Error("ga4 relay exited with error", "err", err)
		return 1
	}
	return 0
}
