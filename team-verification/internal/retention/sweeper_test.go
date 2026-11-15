package retention_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-verification/internal/objects"
	"github.com/buidangphuc/team-verification/internal/retention"
	"github.com/buidangphuc/team-verification/migrations"
)

var now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

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

type seedSub struct {
	id, user, status string
	reviewedAgo      time.Duration
	supersededAgo    time.Duration
	keys             []string
}

func seed(t *testing.T, pool *pgxpool.Pool, objs *objects.Fake, s seedSub) {
	t.Helper()
	ctx := context.Background()
	var reviewed, superseded *time.Time
	if s.reviewedAgo > 0 {
		v := now.Add(-s.reviewedAgo)
		reviewed = &v
	}
	if s.supersededAgo > 0 {
		v := now.Add(-s.supersededAgo)
		superseded = &v
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO kyc_submissions (id, user_id, doc_type, doc_ref, status, reviewed_at, created_at, superseded_at)
		VALUES ($1, $2, 'id_card', $3, $4, $5, $6, $7)`,
		s.id, s.user, s.keys[0], s.status, reviewed, now.Add(-200*day), superseded); err != nil {
		t.Fatalf("seed submission: %v", err)
	}
	for i, k := range s.keys {
		if _, err := pool.Exec(ctx, `
			INSERT INTO kyc_documents (submission_id, doc_index, object_key, content_type, size_bytes)
			VALUES ($1, $2, $3, 'image/jpeg', 100)`, s.id, i, k); err != nil {
			t.Fatalf("seed document: %v", err)
		}
		objs.Put(k, "image/jpeg", 100)
	}
}

func deletedAt(t *testing.T, pool *pgxpool.Pool, key string) *time.Time {
	t.Helper()
	var d *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT deleted_at FROM kyc_documents WHERE object_key=$1`, key).Scan(&d); err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	return d
}

func status(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM kyc_submissions WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return s
}

func newSweeper(pool *pgxpool.Pool, objs objects.Store) *retention.Sweeper {
	return retention.NewSweeper(pool, objs, retention.Config{Retention: 90 * day}, slog.New(slog.DiscardHandler))
}

func TestScenarioRejectedDocumentsAreDeletedAfter90Days(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seed(t, pool, objs, seedSub{id: "kyc-r", user: "u-1", status: "REJECTED", reviewedAgo: 91 * day, keys: []string{"kyc/u-1/a", "kyc/u-1/b"}})

	if _, err := newSweeper(pool, objs).SweepOnce(context.Background(), now); err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	for _, k := range []string{"kyc/u-1/a", "kyc/u-1/b"} {
		if objs.Has(k) {
			t.Errorf("object %s still exists after sweep", k)
		}
		if deletedAt(t, pool, k) == nil {
			t.Errorf("document %s not marked deleted", k)
		}
	}
	if got := status(t, pool, "kyc-r"); got != "REJECTED" {
		t.Fatalf("status = %s, want REJECTED", got)
	}
}

func TestScenarioDocumentsInsideTheWindowAreKept(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seed(t, pool, objs, seedSub{id: "kyc-r", user: "u-1", status: "REJECTED", reviewedAgo: 89 * day, keys: []string{"kyc/u-1/a"}})
	seed(t, pool, objs, seedSub{id: "kyc-v", user: "u-2", status: "VERIFIED", reviewedAgo: 400 * day, keys: []string{"kyc/u-2/a"}})
	seed(t, pool, objs, seedSub{id: "kyc-p", user: "u-3", status: "PENDING", keys: []string{"kyc/u-3/a"}})
	seed(t, pool, objs, seedSub{id: "kyc-s", user: "u-4", status: "VERIFIED", reviewedAgo: 200 * day, supersededAgo: 89 * day, keys: []string{"kyc/u-4/a"}})

	if _, err := newSweeper(pool, objs).SweepOnce(context.Background(), now); err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	for _, k := range []string{"kyc/u-1/a", "kyc/u-2/a", "kyc/u-3/a", "kyc/u-4/a"} {
		if !objs.Has(k) {
			t.Errorf("object %s was deleted inside the retention window", k)
		}
		if deletedAt(t, pool, k) != nil {
			t.Errorf("document %s marked deleted inside the retention window", k)
		}
	}
}

func TestScenarioSupersededDocumentsAreDeletedAfter90Days(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seed(t, pool, objs, seedSub{id: "kyc-old", user: "u-1", status: "VERIFIED", reviewedAgo: 150 * day, supersededAgo: 91 * day, keys: []string{"kyc/u-1/old"}})
	seed(t, pool, objs, seedSub{id: "kyc-new", user: "u-1", status: "PENDING", keys: []string{"kyc/u-1/new"}})

	if _, err := newSweeper(pool, objs).SweepOnce(context.Background(), now); err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if objs.Has("kyc/u-1/old") || deletedAt(t, pool, "kyc/u-1/old") == nil {
		t.Errorf("superseded document was not deleted")
	}
	if !objs.Has("kyc/u-1/new") || deletedAt(t, pool, "kyc/u-1/new") != nil {
		t.Errorf("newer submission's document was deleted")
	}
}

func TestSweeperRetriesFailedObjectDelete(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seed(t, pool, objs, seedSub{id: "kyc-r", user: "u-1", status: "REJECTED", reviewedAgo: 91 * day, keys: []string{"kyc/u-1/a"}})
	sw := newSweeper(pool, objs)

	objs.SetError(errors.New("storage down"))
	if _, err := sw.SweepOnce(context.Background(), now); err == nil {
		t.Fatal("SweepOnce with failing storage returned nil error")
	}
	if deletedAt(t, pool, "kyc/u-1/a") != nil {
		t.Fatal("row marked deleted although the object delete failed")
	}

	objs.SetError(nil)
	if _, err := sw.SweepOnce(context.Background(), now.Add(time.Hour)); err != nil {
		t.Fatalf("retry SweepOnce: %v", err)
	}
	if objs.Has("kyc/u-1/a") || deletedAt(t, pool, "kyc/u-1/a") == nil {
		t.Fatal("next run did not delete the object and mark the row")
	}
	if _, err := sw.SweepOnce(context.Background(), now.Add(2*time.Hour)); err != nil {
		t.Fatalf("third SweepOnce must be a no-op: %v", err)
	}
}
