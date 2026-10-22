package migrate_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

func fsys(up string) fstest.MapFS {
	return fstest.MapFS{
		"0001_init.up.sql":   {Data: []byte(up)},
		"0001_init.down.sql": {Data: []byte("DROP TABLE widgets;")},
	}
}

const goodUp = "CREATE TABLE widgets (id int PRIMARY KEY);"

func TestUpIsIdempotent(t *testing.T) {
	url := testkit.Postgres(t)
	ctx := context.Background()

	if err := migrate.Up(ctx, url, fsys(goodUp)); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := migrate.Up(ctx, url, fsys(goodUp)); err != nil {
		t.Fatalf("second Up must be a no-op, got: %v", err)
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var reg *string
	if err := conn.QueryRow(ctx, "SELECT to_regclass('public.widgets')::text").Scan(&reg); err != nil || reg == nil {
		t.Fatalf("widgets table missing: %v", err)
	}
}

func TestUpFailsOnInvalidSQL(t *testing.T) {
	url := testkit.Postgres(t)

	if err := migrate.Up(context.Background(), url, fsys("CREATE TABEL nope;")); err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}
