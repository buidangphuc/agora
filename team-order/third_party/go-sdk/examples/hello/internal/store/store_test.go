package store_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/migrations"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func TestMigrationsApply(t *testing.T) {
	url := testkit.Postgres(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM greetings").Scan(&n); err != nil {
		t.Fatalf("greetings table missing: %v", err)
	}
}
