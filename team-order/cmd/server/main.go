package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-order/internal/app"
	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/migrations"
)

type migrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func (s migrateSettings) Validate() error {
	if strings.TrimSpace(s.Postgres.URL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return s.Runtime.Validate()
}

func migrateUp(ctx context.Context) error {
	s, err := sdkconfig.Load[migrateSettings]()
	if err != nil {
		return err
	}
	if err := migrate.Up(ctx, s.Postgres.URL, fs.FS(migrations.FS)); err != nil {
		host := "unknown"
		if u, perr := url.Parse(s.Postgres.URL); perr == nil && u.Host != "" {
			host = u.Host
		}
		return fmt.Errorf("migrate %s: %w", host, err)
	}
	return nil
}

func main() {
	if len(os.Args) > 1 && (len(os.Args) != 3 || os.Args[1] != "migrate" || os.Args[2] != "up") {
		fmt.Fprintln(os.Stderr, "usage: server [migrate up]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) == 3 {
		if err := migrateUp(ctx); err != nil {
			stop()
			log.Fatal(err)
		}
		return
	}
	s, err := config.Load()
	if err != nil {
		stop()
		log.Fatal(err)
	}
	if err := app.Run(ctx, s); err != nil {
		stop()
		log.Fatal(err)
	}
}
