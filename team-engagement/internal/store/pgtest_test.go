package store

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-engagement/migrations"
)

// testPool returns a pool on a freshly migrated database: TEST_DATABASE_URL when
// set (a disposable database; its public schema is reset, so never point it at a
// shared one), otherwise a Postgres container from go-sdk testkit. Without Docker
// the test is skipped, or fails when GO_SDK_REQUIRE_DOCKER is set (make check).
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	url := pgURL(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return pool
}

// testOutbox is the outbox store the repositories under test enqueue into.
func testOutbox(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	st, err := outbox.NewStore(pool, OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func outboxTypes(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_type FROM `+OutboxTable+` ORDER BY created_at, event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.TrimPrefix(s, "platform.engagement.v1."))
	}
	return out
}
