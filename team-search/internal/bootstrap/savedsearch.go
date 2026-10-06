package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/repository"
)

// dbOpener opens (and verifies) a Postgres handle for the given URL.
type dbOpener func(ctx context.Context, url string) (*sql.DB, error)

// OpenSavedSearchRepository returns the Postgres-backed repository when
// DATABASE_ENABLED=true, otherwise the in-memory fallback. The returned func
// releases the DB handle (a no-op for the fallback).
func OpenSavedSearchRepository(ctx context.Context, s *config.Settings, logger *slog.Logger) (repository.SavedSearchRepository, func(), error) {
	return selectSavedSearchRepository(ctx, s, logger, openPostgres)
}

func selectSavedSearchRepository(ctx context.Context, s *config.Settings, logger *slog.Logger, open dbOpener) (repository.SavedSearchRepository, func(), error) {
	if !s.Database.Enabled {
		logger.Warn("DATABASE_ENABLED=false: saved searches use an in-memory store and are lost on restart")
		return repository.NewInMemorySavedSearchRepository(), func() {}, nil
	}
	db, err := open(ctx, s.Database.URL)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("saved searches backed by postgres")
	return repository.NewPostgresSavedSearchRepository(db), func() { _ = db.Close() }, nil
}

func openPostgres(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}
