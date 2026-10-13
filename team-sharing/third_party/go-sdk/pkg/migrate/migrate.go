package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	gomigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5 scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Up applies every pending migration found at the root of fsys (use fs.Sub for an
// embedded "migrations" directory). databaseURL is a normal postgres:// URL.
// Having nothing to apply is not an error.
func Up(ctx context.Context, databaseURL string, fsys fs.FS) error {
	src, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	m, err := gomigrate.NewWithSourceInstance("iofs", src, toPgx5(databaseURL))
	if err != nil {
		_ = src.Close()
		return fmt.Errorf("init migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	// golang-migrate takes no context; GracefulStop stops it between migrations.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			m.GracefulStop <- true
		case <-done:
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, gomigrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func toPgx5(u string) string {
	for _, p := range []string{"postgresql://", "postgres://"} {
		if rest, ok := strings.CutPrefix(u, p); ok {
			return "pgx5://" + rest
		}
	}
	return u
}
