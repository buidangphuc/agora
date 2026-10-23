package app

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/migrations"
)

func Migrate(ctx context.Context) error {
	s, err := config.LoadMigrate()
	if err != nil {
		return err
	}
	if err := migrate.Up(ctx, s.Postgres.URL, fs.FS(migrations.FS)); err != nil {
		return fmt.Errorf("migrate %s: %w", s.Postgres.URL, err)
	}
	return nil
}
