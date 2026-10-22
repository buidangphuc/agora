package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/internal/store/fake"
	"github.com/buidangphuc/team-referral/migrations"
)

var (
	_ store.ReferralRepository = (*store.PostgresReferralRepo)(nil)
	_ store.ReferralRepository = (*fake.InMemoryReferralRepo)(nil)
)

func openPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Postgres(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return openPool(t, url)
}

func TestMigrationsApplyAndPostgresRoundtrip(t *testing.T) {
	ctx := context.Background()
	pool := migratedPool(t)
	r := store.NewPostgresReferralRepo(pool)

	if _, err := r.GetCodeByUser(ctx, "alice"); !errors.Is(err, store.ErrCodeNotFound) {
		t.Fatalf("GetCodeByUser before create = %v, want ErrCodeNotFound", err)
	}
	code, err := r.CreateCode(ctx, "alice", "REFAAAA1111")
	if err != nil || code != "REFAAAA1111" {
		t.Fatalf("CreateCode = %q, %v", code, err)
	}
	again, err := r.CreateCode(ctx, "alice", "REFOTHER")
	if err != nil || again != "REFAAAA1111" {
		t.Fatalf("second CreateCode = %q, %v; want the first code", again, err)
	}
	got, err := r.GetCodeByUser(ctx, "alice")
	if err != nil || got != "REFAAAA1111" {
		t.Fatalf("GetCodeByUser = %q, %v", got, err)
	}
	owner, err := r.OwnerOfCode(ctx, "REFAAAA1111")
	if err != nil || owner != "alice" {
		t.Fatalf("OwnerOfCode = %q, %v", owner, err)
	}
	if _, err := r.OwnerOfCode(ctx, "NOPE"); !errors.Is(err, store.ErrCodeNotFound) {
		t.Fatalf("OwnerOfCode unknown = %v, want ErrCodeNotFound", err)
	}

	if err := r.CreateReferral(ctx, "alice", "bob", "REFAAAA1111"); err != nil {
		t.Fatalf("CreateReferral: %v", err)
	}
	if n, err := r.CountReferralsByReferrer(ctx, "alice"); err != nil || n != 1 {
		t.Fatalf("CountReferralsByReferrer = %d, %v", n, err)
	}
	if ok, err := r.HasRedeemed(ctx, "bob"); err != nil || !ok {
		t.Fatalf("HasRedeemed(bob) = %v, %v", ok, err)
	}
	if ok, err := r.HasRedeemed(ctx, "carol"); err != nil || ok {
		t.Fatalf("HasRedeemed(carol) = %v, %v", ok, err)
	}

	rw, err := r.AddReward(ctx, "alice", 10000, "invitee_joined")
	if err != nil {
		t.Fatalf("AddReward: %v", err)
	}
	if rw.GetAmount() != 10000 || rw.GetReason() != "invitee_joined" || rw.GetId() == "" {
		t.Fatalf("reward = %+v", rw)
	}
	if _, err := r.AddReward(ctx, "alice", 5, "bonus"); err != nil {
		t.Fatalf("AddReward 2: %v", err)
	}
	if _, err := r.AddReward(ctx, "zed", 99, "other"); err != nil {
		t.Fatalf("AddReward zed: %v", err)
	}
	if sum, err := r.SumRewards(ctx, "alice"); err != nil || sum != 10005 {
		t.Fatalf("SumRewards(alice) = %d, %v; want 10005", sum, err)
	}
	list, err := r.ListRewards(ctx, "alice", 10, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRewards = %d items, %v; want 2 (zed isolated)", len(list), err)
	}
	page, err := r.ListRewards(ctx, "alice", 1, 1)
	if err != nil || len(page) != 1 {
		t.Fatalf("ListRewards limit1 offset1 = %d items, %v", len(page), err)
	}
}

func TestPostgresRedeemTwiceIsAlreadyRedeemed(t *testing.T) {
	ctx := context.Background()
	pool := migratedPool(t)
	r := store.NewPostgresReferralRepo(pool)

	if err := r.CreateReferral(ctx, "alice", "bob", "REFAAAA1111"); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	err := r.CreateReferral(ctx, "carol", "bob", "REFCCCC2222")
	if !errors.Is(err, store.ErrAlreadyRedeemed) {
		t.Fatalf("second redeem = %v, want ErrAlreadyRedeemed", err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM referrals WHERE referee_id = $1`, "bob").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("referrals rows for bob = %d, want 1", rows)
	}
}

func TestFakeRedeemTwiceIsAlreadyRedeemed(t *testing.T) {
	ctx := context.Background()
	r := fake.NewInMemoryReferralRepo()
	if err := r.CreateReferral(ctx, "alice", "bob", "REFAAAA1111"); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if err := r.CreateReferral(ctx, "carol", "bob", "REFCCCC2222"); !errors.Is(err, store.ErrAlreadyRedeemed) {
		t.Fatalf("second redeem = %v, want store.ErrAlreadyRedeemed", err)
	}
}

func TestNewCodeShape(t *testing.T) {
	c := store.NewCode()
	if !strings.HasPrefix(c, "REF") || len(c) != 11 {
		t.Fatalf("NewCode = %q, want REF + 8 chars", c)
	}
}

func TestProductionCodeCannotReachInMemoryRepository(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "repository")); !os.IsNotExist(err) {
		t.Fatalf("internal/repository must be deleted (stat err = %v)", err)
	}
	root := filepath.Join("..", "..")
	fakeDir, _ := filepath.Abs(filepath.Join("fake"))
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "generated" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		abs, _ := filepath.Abs(p)
		if strings.HasPrefix(abs, fakeDir+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "InMemoryReferralRepo struct") {
			t.Errorf("%s defines an in-memory repository outside internal/store/fake", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
