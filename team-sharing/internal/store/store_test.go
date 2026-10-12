package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-sharing/internal/store"
	"github.com/buidangphuc/team-sharing/migrations"
)

// The root migrations package must apply cleanly, and the Postgres store must
// work against the schema they create.
func TestMigrationsApplyAndPostgresRoundtrip(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, fs.FS(migrations.Files)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgx.Open(ctx, config.Postgres{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := store.NewPostgresShareLinkRepo(pool)

	created, err := repo.Create(ctx, &store.ShareLink{ShortCode: "abc1234", TargetType: "listing", TargetID: "l1", CreatedBy: "user-1", UTM: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ClickCount != 0 {
		t.Fatalf("click_count after create = %d", created.ClickCount)
	}
	if _, err := repo.Create(ctx, &store.ShareLink{ShortCode: "abc1234", TargetType: "listing", TargetID: "l2"}); !errors.Is(err, store.ErrShortCodeExists) {
		t.Fatalf("duplicate code: err = %v, want ErrShortCodeExists", err)
	}
	got, err := repo.Resolve(ctx, "abc1234")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ClickCount != 1 || got.CreatedBy != "user-1" || got.UTM["k"] != "v" {
		t.Fatalf("resolved = %+v", got)
	}
	if _, err := repo.Resolve(ctx, "nope"); !errors.Is(err, store.ErrShareLinkNotFound) {
		t.Fatalf("unknown code: err = %v, want ErrShareLinkNotFound", err)
	}
}
