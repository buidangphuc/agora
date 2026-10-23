package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-verification/internal/app"
	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) == 3 && os.Args[1] == "migrate" && os.Args[2] == "up" {
		url := os.Getenv("DATABASE_URL")
		if url == "" {
			log.Fatal("DATABASE_URL is required")
		}
		if err := migrate.Up(ctx, url, migrations.Files); err != nil {
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
