package store_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/store"
	"github.com/buidangphuc/team-audit/migrations"
)

// The root migrations package must apply cleanly, and the Postgres store must
// work against the schema they create.
func TestMigrationsApplyAndPostgresRoundtrip(t *testing.T) {
	ctx := context.Background()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(ctx, url, fs.FS(migrations.Files)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := postgres.Open(ctx, config.Postgres{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := store.NewPostgresAuditRepo(pool)

	for _, a := range []string{"a1", "a2", "a3"} {
		stored, err := repo.Append(ctx, &auditv1.AuditEvent{
			ActorId: "seller_1", Action: a, TargetType: "listing", TargetId: a, Metadata: map[string]string{"k": "v"},
		})
		if err != nil {
			t.Fatalf("append %s: %v", a, err)
		}
		if stored.GetId() == "" || stored.GetCreatedAt() == nil {
			t.Fatalf("append %s: missing server id or created_at: %+v", a, stored)
		}
	}
	if _, err := repo.Append(ctx, &auditv1.AuditEvent{ActorId: "seller_2", Action: "other", TargetType: "payout"}); err != nil {
		t.Fatalf("append other: %v", err)
	}

	page, total, err := repo.Query(ctx, store.QueryFilter{ActorID: "seller_1", Limit: 2})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 3 || len(page) != 2 || page[0].GetAction() != "a3" || page[1].GetAction() != "a2" {
		t.Fatalf("page 1 = %v (total %d), want newest-first [a3 a2] of 3", page, total)
	}
	if page[0].GetMetadata()["k"] != "v" {
		t.Fatalf("metadata not roundtripped: %v", page[0].GetMetadata())
	}
	page, _, err = repo.Query(ctx, store.QueryFilter{ActorID: "seller_1", Limit: 2, Offset: 2})
	if err != nil || len(page) != 1 || page[0].GetAction() != "a1" {
		t.Fatalf("page 2 = %v, %v, want [a1]", page, err)
	}
	byTarget, total, err := repo.Query(ctx, store.QueryFilter{TargetType: "payout"})
	if err != nil || total != 1 || len(byTarget) != 1 {
		t.Fatalf("by target = %v total %d err %v, want 1 payout event", byTarget, total, err)
	}
}
