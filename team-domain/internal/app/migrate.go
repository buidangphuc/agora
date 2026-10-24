package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-domain/migrations"
)

type migrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func (s migrateSettings) Validate() error {
	if strings.TrimSpace(s.Postgres.URL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	return s.Runtime.Validate()
}

func Migrate(ctx context.Context) error {
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
