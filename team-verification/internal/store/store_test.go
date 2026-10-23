package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
	"github.com/buidangphuc/team-verification/migrations"
)

var (
	_ store.KycRepository = (*store.PostgresKycRepo)(nil)
	_ store.KycRepository = (*storetest.InMemoryKycRepo)(nil)
)

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func pending(id, user string) *store.Submission {
	return &store.Submission{ID: id, UserID: user, DocType: "id_card", DocRef: "ref/" + id, Status: store.StatusPending}
}

func TestPostgresSubmitReviewRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := store.NewPostgresKycRepo(migratedPool(t))

	created, err := repo.Create(ctx, pending("kyc_1", "u-1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != store.StatusPending || created.ReviewedAt != nil || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	reviewed, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusVerified, at)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if reviewed.Status != store.StatusVerified || reviewed.ReviewedAt == nil || !reviewed.ReviewedAt.Equal(at) {
		t.Fatalf("reviewed = %+v", reviewed)
	}

	latest, err := repo.GetLatestByUser(ctx, "u-1")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != "kyc_1" || latest.Status != store.StatusVerified || latest.DocType != "id_card" ||
		latest.DocRef != "ref/kyc_1" || latest.ReviewedAt == nil || !latest.ReviewedAt.Equal(at) ||
		!latest.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("latest = %+v, want same row as reviewed %+v", latest, reviewed)
	}
}

func TestPostgresUpdateStatusIsFinal(t *testing.T) {
	ctx := context.Background()
	repo := store.NewPostgresKycRepo(migratedPool(t))
	if _, err := repo.Create(ctx, pending("kyc_1", "u-1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	first := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	got, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusVerified, first)
	if err != nil || got.Status != store.StatusVerified {
		t.Fatalf("first review = %+v, %v", got, err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_1", store.StatusRejected, first.Add(time.Hour)); !errors.Is(err, store.ErrAlreadyReviewed) {
		t.Fatalf("second review err = %v, want ErrAlreadyReviewed", err)
	}
	stored, err := repo.GetByID(ctx, "kyc_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status != store.StatusVerified || stored.ReviewedAt == nil || !stored.ReviewedAt.Equal(first) {
		t.Fatalf("first decision overwritten: %+v", stored)
	}
}

func TestPostgresUpdateStatusConcurrentSingleWinner(t *testing.T) {
	ctx := context.Background()
	repo := store.NewPostgresKycRepo(migratedPool(t))
	if _, err := repo.Create(ctx, pending("kyc_1", "u-1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status := store.StatusVerified
			if i%2 == 1 {
				status = store.StatusRejected
			}
			_, errs[i] = repo.UpdateStatus(ctx, "kyc_1", status, time.Now())
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, store.ErrAlreadyReviewed):
			t.Fatalf("unexpected error %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want 1", wins)
	}
}

func TestPostgresNotFound(t *testing.T) {
	ctx := context.Background()
	repo := store.NewPostgresKycRepo(migratedPool(t))

	if _, err := repo.GetByID(ctx, "kyc_missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetByID err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetLatestByUser(ctx, "never-submitted"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetLatestByUser err = %v, want ErrNotFound", err)
	}
	if _, err := repo.UpdateStatus(ctx, "kyc_missing", store.StatusVerified, time.Now()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdateStatus err = %v, want ErrNotFound", err)
	}
}

func TestPostgresStatusIsPerUser(t *testing.T) {
	ctx := context.Background()
	repo := store.NewPostgresKycRepo(migratedPool(t))
	if _, err := repo.Create(ctx, pending("kyc_1", "u-1")); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := repo.GetLatestByUser(ctx, "u-2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("u-2 err = %v, want ErrNotFound", err)
	}
	got, err := repo.GetLatestByUser(ctx, "u-1")
	if err != nil || got.ID != "kyc_1" || got.Status != store.StatusPending || got.ReviewedAt != nil {
		t.Fatalf("u-1 row changed: %+v, %v", got, err)
	}
}

func TestPostgresLatestByCreatedAt(t *testing.T) {
	ctx := context.Background()
	pool := migratedPool(t)
	repo := store.NewPostgresKycRepo(pool)
	t1 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Hour)

	insert := func(id string, at time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO kyc_submissions (id, user_id, doc_type, doc_ref, status, created_at)
			 VALUES ($1, 'u-1', 'id_card', 'ref', 'PENDING', $2)`, id, at)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	insert("kyc_late", t2)
	insert("kyc_early", t1)

	got, err := repo.GetLatestByUser(ctx, "u-1")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if got.ID != "kyc_late" || !got.CreatedAt.Equal(t2) {
		t.Fatalf("latest = %s at %v, want kyc_late at %v", got.ID, got.CreatedAt, t2)
	}
}
