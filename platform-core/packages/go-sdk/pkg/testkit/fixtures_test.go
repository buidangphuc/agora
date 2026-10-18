package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func schemaCount(t *testing.T, dsn string) (rows int) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM items").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	return rows
}

func TestSchemaIsolation(t *testing.T) {
	base := testkit.SharedPostgres(t)
	if base != testkit.SharedPostgres(t) {
		t.Fatal("SharedPostgres must return the same server within one binary")
	}
	ctx := context.Background()
	var names []string
	dsns := make([]string, 2)
	for i := range dsns {
		dsns[i] = testkit.Schema(t, base)
	}
	for i, dsn := range dsns {
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatalf("connect %d: %v", i, err)
		}
		if _, err := conn.Exec(ctx, "CREATE TABLE items (id int)"); err != nil {
			t.Fatalf("create table %d: %v", i, err)
		}
		for j := 0; j <= i; j++ {
			if _, err := conn.Exec(ctx, "INSERT INTO items VALUES (1)"); err != nil {
				t.Fatal(err)
			}
		}
		var schema string
		if err := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
			t.Fatal(err)
		}
		names = append(names, schema)
		conn.Close(ctx)
	}
	if names[0] == names[1] {
		t.Fatalf("both DSNs share schema %q", names[0])
	}
	if a, b := schemaCount(t, dsns[0]), schemaCount(t, dsns[1]); a != 1 || b != 2 {
		t.Fatalf("rows = %d and %d, want 1 and 2", a, b)
	}

	// Cleanup drops the schemas: a subtest's Schema is gone once the subtest ends.
	var sub string
	t.Run("sub", func(t *testing.T) {
		sub = testkit.Schema(t, base)
	})
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM information_schema.schemata WHERE schema_name LIKE 'testkit_%'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 { // only the two still-live schemas of this test
		t.Fatalf("testkit schemas = %d, want 2 (subtest schema %q should be dropped)", n, sub)
	}
}

func TestProduceReadAllRoundTrip(t *testing.T) {
	brokers := testkit.Redpanda(t)
	testkit.Produce(t, brokers, "testkit.roundtrip", "k1", []byte("v1"))
	testkit.Produce(t, brokers, "testkit.roundtrip", "k2", []byte("v2"))

	got := testkit.ReadAll(t, brokers, "testkit.roundtrip", 2, 30*time.Second)
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	want := map[string]string{"k1": "v1", "k2": "v2"}
	for _, r := range got {
		if want[string(r.Key)] != string(r.Value) {
			t.Fatalf("record %s=%s unexpected", r.Key, r.Value)
		}
	}
}
