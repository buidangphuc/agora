// Command server is the gRPC entrypoint for team-engagement (EngagementService). It
// owns the process: signals, settings and the exit code. Everything else is in
// internal/app.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-engagement/internal/app"
	"github.com/buidangphuc/team-engagement/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(ctx, s); err != nil {
		log.Fatal(err)
	}
}
