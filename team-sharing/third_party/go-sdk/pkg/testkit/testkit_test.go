package testkit_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func TestPostgresFixtureIsReachable(t *testing.T) {
	url := testkit.Postgres(t)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var one int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("SELECT 1 = %d, err %v", one, err)
	}
}
