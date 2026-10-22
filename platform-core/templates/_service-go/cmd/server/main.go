// Command server is the gRPC entrypoint for this service. main owns the
// process; everything else lives in internal/app.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/your-org/team-service/internal/app"
	"github.com/your-org/team-service/internal/config"
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
