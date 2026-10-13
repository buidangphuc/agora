package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/migrations"
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
	repo := store.NewPostgresReferralRepo(pool)

	code, err := repo.CreateCode(ctx, "alice", "REFabc12345")
	if err != nil || code != "REFabc12345" {
		t.Fatalf("CreateCode = %q, %v", code, err)
	}
	if got, err := repo.GetCodeByUser(ctx, "alice"); err != nil || got != code {
		t.Fatalf("GetCodeByUser = %q, %v", got, err)
	}
	if _, err := repo.GetCodeByUser(ctx, "nobody"); !errors.Is(err, store.ErrCodeNotFound) {
		t.Fatalf("GetCodeByUser(unknown) err = %v, want ErrCodeNotFound", err)
	}
	if owner, err := repo.OwnerOfCode(ctx, code); err != nil || owner != "alice" {
		t.Fatalf("OwnerOfCode = %q, %v", owner, err)
	}

	if err := repo.CreateReferral(ctx, "alice", "bob", code); err != nil {
		t.Fatalf("CreateReferral: %v", err)
	}
	if err := repo.CreateReferral(ctx, "alice", "bob", code); !errors.Is(err, store.ErrAlreadyRedeemed) {
		t.Fatalf("second CreateReferral err = %v, want ErrAlreadyRedeemed", err)
	}
	if n, err := repo.CountReferralsByReferrer(ctx, "alice"); err != nil || n != 1 {
		t.Fatalf("CountReferralsByReferrer = %d, %v", n, err)
	}

	if _, err := repo.AddReward(ctx, "alice", 10000, "invitee_joined"); err != nil {
		t.Fatalf("AddReward: %v", err)
	}
	if total, err := repo.SumRewards(ctx, "alice"); err != nil || total != 10000 {
		t.Fatalf("SumRewards = %d, %v", total, err)
	}
	rewards, err := repo.ListRewards(ctx, "alice", 10, 0)
	if err != nil || len(rewards) != 1 || rewards[0].GetAmount() != 10000 {
		t.Fatalf("ListRewards = %+v, %v", rewards, err)
	}
}
