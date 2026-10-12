// Command hello is the reference Go service.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
