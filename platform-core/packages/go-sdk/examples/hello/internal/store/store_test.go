package store_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/store"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func TestMigrationsApply(t *testing.T) {
	url := testkit.Postgres(t)
	fsys, err := store.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(context.Background(), url, fsys); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgx.Open(context.Background(), config.Postgres{Enabled: true, URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM greetings").Scan(&n); err != nil {
		t.Fatalf("greetings table missing: %v", err)
	}
}
