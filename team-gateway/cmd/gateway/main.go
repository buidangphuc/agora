// Command gateway is the edge: it serves the platform contracts over Connect
// (gRPC + gRPC-web + JSON) so browsers/native apps can reach them, and forwards
// every call to the upstream gRPC services. It holds no business logic and no
// DB (Rules 1-3): it only routes. This file owns the process: signals, settings
// and the exit code; internal/app wires everything.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-gateway/internal/app"
	"github.com/buidangphuc/team-gateway/internal/config"
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
