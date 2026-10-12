// Command server is the gRPC query API for team-search (SearchService:
// SearchListings + Suggest) over the OpenSearch read-model. main owns the process
// (signals, settings, exit code) and everything else lives in internal/app.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-search/internal/app"
	"github.com/buidangphuc/team-search/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.RunServer(ctx, s); err != nil {
		log.Fatal(err)
	}
}
