// Command hello is the reference Go service: main owns the process (signals,
// settings, exit code) and everything else lives in internal/app.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/examples/hello/internal/app"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/examples/hello/internal/config"
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
