package app

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-promotion/internal/config"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func Migrate(ctx context.Context, s config.MigrateSettings) error {
	files, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := migrate.Up(ctx, s.Postgres.URL, files); err != nil {
		host := "unknown"
		if u, perr := url.Parse(s.Postgres.URL); perr == nil && u.Host != "" {
			host = u.Host
		}
		return fmt.Errorf("migrate %s: %w", host, err)
	}
	return nil
}
