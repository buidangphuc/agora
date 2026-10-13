// Command indexer is the Kafka consumer that keeps the OpenSearch read-model in
// sync: it reads listing.events and upserts/deletes documents (ADR-0002/0005).
// main owns the process (signals, settings, exit code) and everything else lives
// in internal/app.
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
	if err := app.RunIndexer(ctx, s); err != nil {
		log.Fatal(err)
	}
}
