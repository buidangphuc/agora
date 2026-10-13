// Command consumer is the analytics warehouse writer: a Kafka consumer that reads analytics.events
// and order.events and appends them to the warehouse (DuckDB local / BigQuery prod)
// behind the WarehouseWriter seam. main owns the process (signals, settings, exit
// code); everything else lives in internal/app.
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
	if err := app.RunConsumer(ctx, s); err != nil {
		log.Fatal(err)
	}
}
