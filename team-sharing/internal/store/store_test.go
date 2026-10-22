package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-sharing/internal/sharelink"
	"github.com/buidangphuc/team-sharing/internal/store"
	"github.com/buidangphuc/team-sharing/migrations"
)

var _ sharelink.Repository = (*store.Postgres)(nil)

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migratedDB(t *testing.T) string {
	t.Helper()
	url := testkit.Postgres(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return url
}

func TestPostgresCreateResolveAcrossInstances(t *testing.T) {
	ctx := context.Background()
	url := migratedDB(t)
	a := store.NewPostgres(openPool(t, url))
	b := store.NewPostgres(openPool(t, url))

	created, err := a.Create(ctx, &sharelink.ShareLink{
		ShortCode: "Ab3dE9x", TargetType: "listing", TargetID: "listing_001",
		CreatedBy: "user-1", UTM: map[string]string{"utm_source": "zalo"},
		OgTitle: "t", OgDescription: "d", OgImageURL: "http://img",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ShortCode != "Ab3dE9x" {
		t.Fatalf("created code = %q", created.ShortCode)
	}

	got, err := b.Resolve(ctx, "Ab3dE9x")
	if err != nil {
		t.Fatalf("resolve on second instance: %v", err)
	}
	if got.TargetType != "listing" || got.TargetID != "listing_001" {
		t.Fatalf("target = %s/%s", got.TargetType, got.TargetID)
	}
	if got.ClickCount != 1 {
		t.Fatalf("click_count = %d, want 1", got.ClickCount)
	}
	if got.CreatedBy != "user-1" || got.UTM["utm_source"] != "zalo" ||
		got.OgTitle != "t" || got.OgDescription != "d" || got.OgImageURL != "http://img" {
		t.Fatalf("stored fields lost: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at not populated")
	}
}

func TestPostgresResolveIncrementsByOneEachCall(t *testing.T) {
	ctx := context.Background()
	r := store.NewPostgres(openPool(t, migratedDB(t)))
	if _, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "cnt0001", TargetType: "x", TargetID: "y"}); err != nil {
		t.Fatal(err)
	}
	for want := int64(1); want <= 3; want++ {
		got, err := r.Resolve(ctx, "cnt0001")
		if err != nil {
			t.Fatal(err)
		}
		if got.ClickCount != want {
			t.Fatalf("click_count = %d, want %d", got.ClickCount, want)
		}
	}
}

func TestPostgresResolveIsAtomicUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	r := store.NewPostgres(openPool(t, migratedDB(t)))
	if _, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "conc001", TargetType: "x", TargetID: "y"}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	counts := make(chan int64, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := r.Resolve(ctx, "conc001")
			if err != nil {
				t.Error(err)
				return
			}
			counts <- got.ClickCount
		}()
	}
	wg.Wait()
	close(counts)
	seen := map[int64]bool{}
	for c := range counts {
		if seen[c] {
			t.Fatalf("click_count %d returned twice: increment not atomic", c)
		}
		seen[c] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d distinct counts, want %d", len(seen), n)
	}
}

func TestPostgresResolveUnknownIsNotFound(t *testing.T) {
	r := store.NewPostgres(openPool(t, migratedDB(t)))
	_, err := r.Resolve(context.Background(), "nope000")
	if !errors.Is(err, sharelink.ErrShareLinkNotFound) {
		t.Fatalf("err = %v, want ErrShareLinkNotFound", err)
	}
}

func TestPostgresCreateDuplicateCodeIsShortCodeExists(t *testing.T) {
	ctx := context.Background()
	r := store.NewPostgres(openPool(t, migratedDB(t)))
	if _, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "dup0001", TargetType: "a", TargetID: "1"}); err != nil {
		t.Fatal(err)
	}
	_, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "dup0001", TargetType: "b", TargetID: "2"})
	if !errors.Is(err, sharelink.ErrShortCodeExists) {
		t.Fatalf("err = %v, want ErrShortCodeExists", err)
	}
	got, err := r.Resolve(ctx, "dup0001")
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetType != "a" || got.TargetID != "1" {
		t.Fatalf("original link overwritten: %+v", got)
	}
}

func TestPostgresNilUTMStoredAsEmptyObject(t *testing.T) {
	ctx := context.Background()
	url := migratedDB(t)
	r := store.NewPostgres(openPool(t, url))
	if _, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "nil0001", TargetType: "a", TargetID: "1"}); err != nil {
		t.Fatalf("create with nil utm: %v", err)
	}
	got, err := r.Resolve(ctx, "nil0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UTM) != 0 {
		t.Fatalf("utm = %v, want empty", got.UTM)
	}
	var raw string
	if err := openPool(t, url).QueryRow(ctx, `SELECT utm::text FROM share_links WHERE short_code='nil0001'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "{}" {
		t.Fatalf("stored utm = %s, want {}", raw)
	}
}

func TestPostgresAnonymousCreatorStoredAsEmpty(t *testing.T) {
	ctx := context.Background()
	r := store.NewPostgres(openPool(t, migratedDB(t)))
	if _, err := r.Create(ctx, &sharelink.ShareLink{ShortCode: "anon001", TargetType: "a", TargetID: "1"}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Resolve(ctx, "anon001")
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedBy != "" {
		t.Fatalf("created_by = %q, want empty", got.CreatedBy)
	}
}

func TestMigrationsApply(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("migrate up on empty database: %v", err)
	}
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("second migrate up must be a no-op: %v", err)
	}
	pool := openPool(t, url)

	wantCols := map[string]bool{
		"short_code": false, "target_type": false, "target_id": false, "utm": false,
		"og_title": false, "og_description": false, "og_image_url": false,
		"click_count": false, "created_at": false, "created_by": false,
	}
	rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name='share_links'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		if _, ok := wantCols[c]; !ok {
			t.Errorf("unexpected column %q", c)
		}
		wantCols[c] = true
	}
	for c, ok := range wantCols {
		if !ok {
			t.Errorf("missing column %q", c)
		}
	}

	var idx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='share_links' AND indexname='idx_share_links_target'`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("idx_share_links_target count = %d, want 1", idx)
	}

	var version int64
	if err := pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want 2", version)
	}
}
