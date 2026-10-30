package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-search/internal/app"
	"github.com/buidangphuc/team-search/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := config.LoadIndexer()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.RunIndexer(ctx, s); err != nil {
		log.Fatal(err)
	}
}
