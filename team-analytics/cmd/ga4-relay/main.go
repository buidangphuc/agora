// Command ga4-relay forwards paid orders to GA4 (Measurement Protocol). main owns the process
// (signals, settings, exit code); everything else lives in internal/app.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.RunGA4Relay(ctx, s); err != nil {
		log.Fatal(err)
	}
}
