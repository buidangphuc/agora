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

	if len(os.Args) == 3 && os.Args[1] == "migrate" && os.Args[2] == "up" {
		s, err := config.LoadMigrate()
		if err != nil {
			log.Fatal(err)
		}
		if err := app.Migrate(ctx, s); err != nil {
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
