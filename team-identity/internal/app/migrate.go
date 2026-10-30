package app

import (
	"context"
	"fmt"
	"net/url"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/store"
)

func Migrate(ctx context.Context, s config.MigrateSettings) error {
	if err := migrate.Up(ctx, s.Postgres.URL, store.Migrations()); err != nil {
		host := "unknown"
		if u, perr := url.Parse(s.Postgres.URL); perr == nil && u.Host != "" {
			host = u.Host
		}
		return fmt.Errorf("migrate %s: %w", host, err)
	}
	return nil
}
