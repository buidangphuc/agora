package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/repository"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func failOpener(context.Context, string) (*sql.DB, error) { panic("opener must not be called") }

func TestSelect_DisabledFallsBackToInMemory(t *testing.T) {
	s := &config.Settings{}
	repo, closeFn, err := selectSavedSearchRepository(context.Background(), s, quiet, failOpener)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if _, ok := repo.(*repository.InMemorySavedSearchRepository); !ok {
		t.Fatalf("got %T, want in-memory", repo)
	}
}

func TestSelect_EnabledUsesPostgres(t *testing.T) {
	s := &config.Settings{Database: config.Database{Enabled: true, URL: "postgres://u:p@h/db"}}
	var gotURL string
	open := func(_ context.Context, url string) (*sql.DB, error) {
		gotURL = url
		return sql.Open("pgx", url) // lazy: no connection made
	}
	repo, closeFn, err := selectSavedSearchRepository(context.Background(), s, quiet, open)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if _, ok := repo.(*repository.PostgresSavedSearchRepository); !ok {
		t.Fatalf("got %T, want postgres", repo)
	}
	if gotURL != s.Database.URL {
		t.Fatalf("opener url = %q", gotURL)
	}
}

func TestSelect_EnabledOpenErrorFailsFast(t *testing.T) {
	s := &config.Settings{Database: config.Database{Enabled: true, URL: "x"}}
	boom := errors.New("down")
	_, _, err := selectSavedSearchRepository(context.Background(), s, quiet,
		func(context.Context, string) (*sql.DB, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom (no silent in-memory fallback when DB is configured)", err)
	}
}
