package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-identity/internal/app"
	"github.com/buidangphuc/team-identity/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if len(os.Args) != 3 || os.Args[2] != "up" {
			fmt.Fprintln(os.Stderr, "usage: server migrate up")
			stop()
			os.Exit(2)
		}
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
