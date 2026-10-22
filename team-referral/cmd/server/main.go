package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-referral/internal/app"
	"github.com/buidangphuc/team-referral/internal/config"
)

func main() {
	s, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, s); err != nil {
		log.Fatal(err)
	}
}
