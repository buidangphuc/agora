package referral_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

var t0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func testConfig() referral.Config {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	return referral.Config{
		RewardXU: 10000, DailyCap: 5, LifetimeCap: 50,
		RedeemMaxAccountAge: 168 * time.Hour, SweepInterval: time.Minute, CapLocation: loc,
	}
}

type fixture struct {
	repo  *fake.InMemoryReferralRepo
	svc   *referral.Service
	clock *clock
	cfg   referral.Config
}

func newFixture(mutate ...func(*referral.Config)) *fixture {
	cfg := testConfig()
	for _, m := range mutate {
		m(&cfg)
	}
	repo := fake.NewInMemoryReferralRepo()
	clk := &clock{now: t0}
	return &fixture{
		repo:  repo,
		clock: clk,
		cfg:   cfg,
		svc:   referral.NewService(repo, referral.WithConfig(cfg), referral.WithClock(clk.Now)),
	}
}

func (f *fixture) sweeper() *referral.Sweeper {
	return referral.NewSweeper(f.repo, referral.WithConfig(f.cfg), referral.WithClock(f.clock.Now))
}

func (f *fixture) register(t testing.TB, user string, at time.Time) {
	t.Helper()
	err := f.repo.ApplyUserRegistered(context.Background(), store.UserRegisteredInput{
		EventID: "ur-" + user, UserID: user, RegisteredAt: at, At: at, LifetimeCap: f.cfg.LifetimeCap,
	})
	if err != nil {
		t.Fatalf("register %s: %v", user, err)
	}
}

func (f *fixture) registerNow(t testing.TB, users ...string) {
	t.Helper()
	for _, u := range users {
		f.register(t, u, t0.Add(-time.Hour))
	}
}

func (f *fixture) invitee(t testing.TB, _, code, referee, order string, releaseAt time.Time) {
	t.Helper()
	ctx := context.Background()
	err := f.repo.ApplyUserRegistered(ctx, store.UserRegisteredInput{
		EventID: "ur-" + referee, UserID: referee, ReferralCode: code, RegisteredAt: t0.Add(-time.Hour), At: t0.Add(-time.Hour), LifetimeCap: 1000,
	})
	if err != nil {
		t.Fatalf("signup %s: %v", referee, err)
	}
	err = f.repo.ApplyOrderCompleted(ctx, store.OrderCompletedInput{
		EventID: "oc-" + order, OrderID: order, BuyerID: referee, CompletedAt: t0.Add(-time.Hour),
		ReleaseAt: releaseAt, At: t0.Add(-time.Hour), RewardXU: 10000, LifetimeCap: 1000,
	})
	if err != nil {
		t.Fatalf("order %s: %v", order, err)
	}
}

func (f *fixture) inviterWithRewards(t testing.TB, inviter string, n int, releaseAt time.Time) string {
	t.Helper()
	code, err := f.svc.CreateCode(context.Background(), inviter)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		f.invitee(t, inviter, code, inviter+"-ref-"+string(rune('a'+i%26))+string(rune('a'+i/26)), inviter+"-ord-"+string(rune('a'+i%26))+string(rune('a'+i/26)), releaseAt)
	}
	return code
}

func newReferralService() *referral.Service {
	return referral.NewService(fake.NewInMemoryReferralRepo())
}

// CreateCode mints a code and GetMyReferral returns that same code (roundtrip),
// with zero stats before anyone redeems it.
func TestCreateThenGetRoundtrip(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	code, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	if code == "" {
		t.Fatal("expected a non-empty referral code")
	}

	if !regexp.MustCompile(`^REF[0-9a-f]{8}$`).MatchString(code) {
		t.Fatalf("code %q does not match ^REF[0-9a-f]{8}$", code)
	}

	// Idempotent: a second CreateCode returns the same code.
	again, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code (again): %v", err)
	}
	if again != code {
		t.Fatalf("expected idempotent code %q, got %q", code, again)
	}

	mr, err := svc.GetMyReferral(ctx, "alice")
	if err != nil {
		t.Fatalf("get my referral: %v", err)
	}
	if mr.Code != code {
		t.Fatalf("roundtrip mismatch: minted %q, got %q", code, mr.Code)
	}
	if mr.InvitedCount != 0 || mr.RewardsTotal != 0 {
		t.Fatalf("expected zero stats before any redemption, got invited=%d total=%d", mr.InvitedCount, mr.RewardsTotal)
	}
}

func TestRedeemAttributesWithoutCredit(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.registerNow(t, "bob")
	code, err := f.svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}

	if err := f.svc.Redeem(ctx, "bob", code); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	alice, err := f.svc.GetMyReferral(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if alice.InvitedCount != 1 || alice.PendingXU != 0 || alice.ReleasedXU != 0 || alice.RewardsTotal != 0 {
		t.Fatalf("alice = %+v, want invited=1 and no xu", alice)
	}
	rewards, _, err := f.svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil || len(rewards) != 0 {
		t.Fatalf("rewards = %+v, %v; want none", rewards, err)
	}
	ref, err := f.repo.GetReferral(ctx, "bob")
	if err != nil || ref.Status != store.StatusAttributed || ref.ReferrerID != "alice" {
		t.Fatalf("referral = %+v, %v", ref, err)
	}
	rows := f.repo.OutboxRows()
	if len(rows) != 1 || rows[0].EventType != "platform.referral.v1.ReferralAttributed" {
		t.Fatalf("outbox = %+v, want one ReferralAttributed", rows)
	}
	if err := f.svc.Redeem(ctx, "bob", code); !errors.Is(err, referral.ErrAlreadyRedeemed) {
		t.Fatalf("expected ErrAlreadyRedeemed on second redeem, got %v", err)
	}
}

func TestRedeemRejectsSelfAndUnknown(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.registerNow(t, "alice", "bob")
	code, err := f.svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	if err := f.svc.Redeem(ctx, "alice", code); !errors.Is(err, referral.ErrSelfReferral) {
		t.Fatalf("expected ErrSelfReferral, got %v", err)
	}
	if err := f.svc.Redeem(ctx, "bob", "REFnope123"); !errors.Is(err, referral.ErrCodeNotFound) {
		t.Fatalf("expected ErrCodeNotFound for unknown code, got %v", err)
	}
	if err := f.svc.Redeem(ctx, "bob", ""); !errors.Is(err, referral.ErrEmptyCode) {
		t.Fatalf("expected ErrEmptyCode for empty code, got %v", err)
	}
	alice, _ := f.svc.GetMyReferral(ctx, "alice")
	if alice.InvitedCount != 0 || alice.PendingXU != 0 || alice.ReleasedXU != 0 {
		t.Fatalf("refused redeems left state: %+v", alice)
	}
	if n := len(f.repo.OutboxRows()); n != 0 {
		t.Fatalf("%d outbox rows after refused redeems", n)
	}
}

// One user's referral data never leaks into another user's view.
func TestCrossUserIsolation(t *testing.T) {
	f := newFixture()
	svc := f.svc
	ctx := context.Background()

	aliceCode, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create alice code: %v", err)
	}
	bobCode, err := svc.CreateCode(ctx, "bob")
	if err != nil {
		t.Fatalf("create bob code: %v", err)
	}
	if aliceCode == bobCode {
		t.Fatal("expected distinct codes per user")
	}

	f.registerNow(t, "carol")
	if err := svc.Redeem(ctx, "carol", aliceCode); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	bob, err := svc.GetMyReferral(ctx, "bob")
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	if bob.InvitedCount != 0 || bob.RewardsTotal != 0 || bob.PendingXU != 0 || bob.ReleasedXU != 0 {
		t.Fatalf("cross-user leak: bob should have zero stats, got %+v", bob)
	}
	bobRewards, _, err := svc.ListRewards(ctx, "bob", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("list bob rewards: %v", err)
	}
	if len(bobRewards) != 0 {
		t.Fatalf("cross-user leak: bob should have no rewards, got %d", len(bobRewards))
	}

	f.inviterWithRewards(t, "alice", 2, t0.Add(48*time.Hour))
	f.inviterWithRewards(t, "bob", 1, t0.Add(48*time.Hour))
	for user, wantRows := range map[string]int{"alice": 2, "bob": 1} {
		mine, err := svc.GetMyReferral(ctx, user)
		if err != nil {
			t.Fatalf("get %s: %v", user, err)
		}
		rows, _, err := svc.ListRewards(ctx, user, &referralv1.ListReferralRewardsRequest{})
		if err != nil || len(rows) != wantRows || mine.PendingXU != int64(wantRows)*10000 {
			t.Fatalf("%s sees %d rows and pending=%d, want %d rows", user, len(rows), mine.PendingXU, wantRows)
		}
	}
}

// Empty user id is rejected everywhere (anonymous callers never reach the DB).
func TestEmptyUserRejected(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	if _, err := svc.CreateCode(ctx, ""); !errors.Is(err, referral.ErrEmptyUser) {
		t.Fatalf("CreateCode empty user: got %v", err)
	}
	if _, err := svc.GetMyReferral(ctx, ""); !errors.Is(err, referral.ErrEmptyUser) {
		t.Fatalf("GetMyReferral empty user: got %v", err)
	}
	if err := svc.Redeem(ctx, "", "REFx"); !errors.Is(err, referral.ErrEmptyUser) {
		t.Fatalf("Redeem empty referee: got %v", err)
	}
	if _, _, err := svc.ListRewards(ctx, "", nil); !errors.Is(err, referral.ErrEmptyUser) {
		t.Fatalf("ListRewards empty user: got %v", err)
	}
}

// ListRewards paginates newest-first and surfaces a cursor for the next page.
func TestListRewardsPagination(t *testing.T) {
	f := newFixture()
	svc := f.svc
	ctx := context.Background()

	f.inviterWithRewards(t, "alice", 5, t0.Add(48*time.Hour))

	page1, cursor, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 rewards on page 1, got %d", len(page1))
	}
	if cursor != "2" {
		t.Fatalf("expected cursor %q after page 1, got %q", "2", cursor)
	}

	page2, cursor2, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2, Cursor: cursor},
	})
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 rewards on page 2, got %d", len(page2))
	}
	if cursor2 != "4" {
		t.Fatalf("expected cursor %q after page 2, got %q", "4", cursor2)
	}

	page3, cursor3, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2, Cursor: cursor2},
	})
	if err != nil {
		t.Fatalf("list page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("expected 1 reward on page 3, got %d", len(page3))
	}
	if cursor3 != "" {
		t.Fatalf("expected no cursor after the last page, got %q", cursor3)
	}

	seen := map[string]bool{}
	for _, rw := range append(append(page1, page2...), page3...) {
		if seen[rw.GetId()] {
			t.Fatalf("reward %s repeated across pages", rw.GetId())
		}
		seen[rw.GetId()] = true
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 distinct rewards, got %d", len(seen))
	}
}

func TestRedeemRejectsSecondRedeem(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.registerNow(t, "alice", "bob")
	aliceCode, err := f.svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create alice code: %v", err)
	}
	carolCode, err := f.svc.CreateCode(ctx, "carol")
	if err != nil {
		t.Fatalf("create carol code: %v", err)
	}
	if err := f.svc.Redeem(ctx, "bob", aliceCode); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if err := f.svc.Redeem(ctx, "bob", carolCode); !errors.Is(err, referral.ErrAlreadyRedeemed) {
		t.Fatalf("expected ErrAlreadyRedeemed redeeming a different code, got %v", err)
	}
	if err := f.svc.Redeem(ctx, "alice", aliceCode); !errors.Is(err, referral.ErrSelfReferral) {
		t.Fatalf("expected ErrSelfReferral, got %v", err)
	}

	alice, err := f.svc.GetMyReferral(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if alice.InvitedCount != 1 || alice.RewardsTotal != 0 || alice.PendingXU != 0 {
		t.Fatalf("alice should keep one referral and no reward, got %+v", alice)
	}
	rewards, _, err := f.svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil || len(rewards) != 0 {
		t.Fatalf("expected no reward for alice, got %d, %v", len(rewards), err)
	}
	carol, err := f.svc.GetMyReferral(ctx, "carol")
	if err != nil {
		t.Fatalf("get carol: %v", err)
	}
	if carol.InvitedCount != 0 {
		t.Fatalf("refused redeem must not count for carol, got %d", carol.InvitedCount)
	}
}

func TestListRewardsPageSizeClamp(t *testing.T) {
	f := newFixture()
	svc := f.svc
	ctx := context.Background()
	f.inviterWithRewards(t, "alice", 150, t0.Add(48*time.Hour))

	for _, tc := range []struct {
		size int32
		want int
	}{{0, 20}, {1000, 100}} {
		got, _, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
			Page: &commonv1.PageRequest{PageSize: tc.size},
		})
		if err != nil {
			t.Fatalf("page size %d: %v", tc.size, err)
		}
		if len(got) != tc.want {
			t.Fatalf("page size %d: expected %d rewards, got %d", tc.size, tc.want, len(got))
		}
	}
}

func TestMalformedCursorStartsAtZero(t *testing.T) {
	f := newFixture()
	svc := f.svc
	ctx := context.Background()
	f.inviterWithRewards(t, "alice", 5, t0.Add(48*time.Hour))

	first, firstCursor, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	for _, cur := range []string{"abc", "-3"} {
		got, gotCursor, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
			Page: &commonv1.PageRequest{PageSize: 2, Cursor: cur},
		})
		if err != nil {
			t.Fatalf("cursor %q returned error: %v", cur, err)
		}
		if gotCursor != firstCursor || len(got) != len(first) {
			t.Fatalf("cursor %q: expected first page (%d rows, cursor %q), got %d rows, cursor %q", cur, len(first), firstCursor, len(got), gotCursor)
		}
		for i := range got {
			if got[i].GetId() != first[i].GetId() {
				t.Fatalf("cursor %q: row %d is %s, expected %s", cur, i, got[i].GetId(), first[i].GetId())
			}
		}
	}
}
