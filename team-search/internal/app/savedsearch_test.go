package app_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"

	"github.com/buidangphuc/team-search/internal/app"
	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/store"
)

var quiet = slog.New(slog.DiscardHandler)

func failOpener(context.Context, sdkconfig.Postgres) (*pgxpool.Pool, error) {
	return nil, errors.New("down")
}

func selectRepo(s config.Settings, open app.PoolOpener) (store.SavedSearchRepository, error) {
	var stack lifecycle.Stack
	defer func() { _ = stack.Close(context.Background()) }()
	return app.SelectSavedSearchRepository(context.Background(), s, quiet, healthx.New(healthx.DefaultInterval, quiet), &stack, open)
}

// Local development stays usable without Postgres: an unreachable database falls
// back to the in-memory store (the old DATABASE_ENABLED=false path).
func TestSelect_UnreachableLocalFallsBackToInMemory(t *testing.T) {
	s := config.Settings{Runtime: sdkconfig.Runtime{Env: "local"}}
	repo, err := selectRepo(s, failOpener)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.(*store.InMemorySavedSearchRepository); !ok {
		t.Fatalf("got %T, want in-memory", repo)
	}
}

func TestSelect_ReachableUsesPostgres(t *testing.T) {
	s := config.Settings{Postgres: sdkconfig.Postgres{URL: "postgres://u:p@h/db"}}
	var gotURL string
	open := func(ctx context.Context, pg sdkconfig.Postgres) (*pgxpool.Pool, error) {
		gotURL = pg.URL
		return pgxpool.New(ctx, pg.URL) // lazy: no connection made
	}
	repo, err := selectRepo(s, open)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.(*store.PostgresSavedSearchRepository); !ok {
		t.Fatalf("got %T, want postgres", repo)
	}
	if gotURL != s.Postgres.URL {
		t.Fatalf("opener url = %q", gotURL)
	}
}

func TestSelect_OpenErrorFailsFast(t *testing.T) {
	s := config.Settings{Postgres: sdkconfig.Postgres{URL: "x"}}
	boom := errors.New("down")
	_, err := selectRepo(s, func(context.Context, sdkconfig.Postgres) (*pgxpool.Pool, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom (no silent in-memory fallback outside local)", err)
	}
}

func TestSelect_StrictEnvRefusesInMemory(t *testing.T) {
	for _, env := range []string{"staging", "production"} {
		s := config.Settings{Runtime: sdkconfig.Runtime{Env: env}}
		repo, err := selectRepo(s, failOpener)
		if err == nil || repo != nil {
			t.Fatalf("ENV=%s unreachable database: got repo=%v err=%v, want a boot error", env, repo, err)
		}
	}
}

func TestSelect_StrictEnvUnreachableDBRefusesToBoot(t *testing.T) {
	s := config.Settings{Runtime: sdkconfig.Runtime{Env: "production"}, Postgres: sdkconfig.Postgres{URL: "x"}}
	if _, err := selectRepo(s, failOpener); err == nil {
		t.Fatal("unreachable database in production must fail the boot")
	}
}
