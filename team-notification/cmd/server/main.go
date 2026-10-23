package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-notification/internal/app"
	"github.com/buidangphuc/team-notification/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) == 3 && os.Args[1] == "migrate" && os.Args[2] == "up" {
		if err := app.Migrate(ctx); err != nil {
			log.Fatal(err)
		}
		return
	}
	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(ctx, s); err != nil {
		log.Fatal(err)
	}
}
