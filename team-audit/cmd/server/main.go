package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-audit/internal/app"
	"github.com/buidangphuc/team-audit/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) == 3 && os.Args[1] == "migrate" && os.Args[2] == "up" {
		if err := app.Migrate(ctx, s); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := app.Run(ctx, s); err != nil {
		log.Fatal(err)
	}
}
