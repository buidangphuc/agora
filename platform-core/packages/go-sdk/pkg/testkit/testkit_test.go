package testkit_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
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

func TestRedpandaFixtureIsReachable(t *testing.T) {
	brokers := testkit.Redpanda(t)

	client, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping %s: %v", brokers, err)
	}
}
