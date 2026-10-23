package app

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-notification/internal/store"
)

type migrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func Migrate(ctx context.Context) error {
	s, err := sdkconfig.Load[migrateSettings]()
	if err != nil {
		return err
	}
	files, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		return err
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
