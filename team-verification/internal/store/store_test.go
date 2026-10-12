package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/migrations"
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
	repo := store.NewPostgresKycRepo(pool)

	created, err := repo.Create(ctx, &store.Submission{ID: "kyc_1", UserID: "u1", DocType: "national_id", DocRef: "ref", Status: store.StatusPending})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != store.StatusPending || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}
	if _, err := repo.GetByID(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown id: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetLatestByUser(ctx, "nobody"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: err = %v, want ErrNotFound", err)
	}

	reviewed, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusVerified, time.Now().UTC())
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if reviewed.Status != store.StatusVerified || reviewed.ReviewedAt == nil {
		t.Fatalf("reviewed = %+v", reviewed)
	}
	// A decision is final: the second review sees no PENDING row.
	if _, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusRejected, time.Now().UTC()); !errors.Is(err, store.ErrAlreadyReviewed) {
		t.Fatalf("second review: err = %v, want ErrAlreadyReviewed", err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_missing", store.StatusVerified, time.Now().UTC()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing review: err = %v, want ErrNotFound", err)
	}
	latest, err := repo.GetLatestByUser(ctx, "u1")
	if err != nil || latest.ID != "kyc_1" || latest.Status != store.StatusVerified {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
}
