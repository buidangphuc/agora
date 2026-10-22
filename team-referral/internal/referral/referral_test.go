package referral_test

import (
	"context"
	"regexp"
	"testing"

	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

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

// Redeem attributes the referee to the referrer: the referrer's invited count
// and reward total move, and a reward-intent row lands on the referrer's ledger.
func TestRedeemAttributesReferee(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	code, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}

	if err := svc.Redeem(ctx, "bob", code); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// Referrer (alice) sees the invite and the mock coin credit.
	alice, err := svc.GetMyReferral(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if alice.InvitedCount != 1 {
		t.Fatalf("expected alice invited=1, got %d", alice.InvitedCount)
	}
	if alice.RewardsTotal != referral.RewardOnReferral {
		t.Fatalf("expected alice rewards=%d, got %d", referral.RewardOnReferral, alice.RewardsTotal)
	}

	rewards, _, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("list rewards: %v", err)
	}
	if len(rewards) != 1 || rewards[0].GetReason() != referral.ReasonInviteeJoined {
		t.Fatalf("expected one %q reward, got %+v", referral.ReasonInviteeJoined, rewards)
	}

	// A double redemption by the same referee is rejected.
	if err := svc.Redeem(ctx, "bob", code); err != referral.ErrAlreadyRedeemed {
		t.Fatalf("expected ErrAlreadyRedeemed on second redeem, got %v", err)
	}
}

// A user cannot redeem their own code, and unknown codes are rejected.
func TestRedeemRejectsSelfAndUnknown(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	code, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	if err := svc.Redeem(ctx, "alice", code); err != referral.ErrSelfReferral {
		t.Fatalf("expected ErrSelfReferral, got %v", err)
	}
	if err := svc.Redeem(ctx, "bob", "REFnope123"); err != referral.ErrCodeNotFound {
		t.Fatalf("expected ErrCodeNotFound for unknown code, got %v", err)
	}
	if err := svc.Redeem(ctx, "bob", ""); err != referral.ErrEmptyCode {
		t.Fatalf("expected ErrEmptyCode for empty code, got %v", err)
	}
}

// One user's referral data never leaks into another user's view.
func TestCrossUserIsolation(t *testing.T) {
	svc := newReferralService()
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

	// carol redeems alice's code; bob must be completely unaffected.
	if err := svc.Redeem(ctx, "carol", aliceCode); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	bob, err := svc.GetMyReferral(ctx, "bob")
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	if bob.InvitedCount != 0 || bob.RewardsTotal != 0 {
		t.Fatalf("cross-user leak: bob should have zero stats, got invited=%d total=%d", bob.InvitedCount, bob.RewardsTotal)
	}
	bobRewards, _, err := svc.ListRewards(ctx, "bob", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("list bob rewards: %v", err)
	}
	if len(bobRewards) != 0 {
		t.Fatalf("cross-user leak: bob should have no rewards, got %d", len(bobRewards))
	}
}

// Empty user id is rejected everywhere (anonymous callers never reach the DB).
func TestEmptyUserRejected(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	if _, err := svc.CreateCode(ctx, ""); err != referral.ErrEmptyUser {
		t.Fatalf("CreateCode empty user: got %v", err)
	}
	if _, err := svc.GetMyReferral(ctx, ""); err != referral.ErrEmptyUser {
		t.Fatalf("GetMyReferral empty user: got %v", err)
	}
	if err := svc.Redeem(ctx, "", "REFx"); err != referral.ErrEmptyUser {
		t.Fatalf("Redeem empty referee: got %v", err)
	}
	if _, _, err := svc.ListRewards(ctx, "", nil); err != referral.ErrEmptyUser {
		t.Fatalf("ListRewards empty user: got %v", err)
	}
}

// ListRewards paginates newest-first and surfaces a cursor for the next page.
func TestListRewardsPagination(t *testing.T) {
	svc := newReferralService()
	ctx := context.Background()

	code, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	for _, referee := range []string{"u1", "u2", "u3", "u4", "u5"} {
		if err := svc.Redeem(ctx, referee, code); err != nil {
			t.Fatalf("redeem %s: %v", referee, err)
		}
	}

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
	svc := newReferralService()
	ctx := context.Background()

	aliceCode, err := svc.CreateCode(ctx, "alice")
	if err != nil {
		t.Fatalf("create alice code: %v", err)
	}
	carolCode, err := svc.CreateCode(ctx, "carol")
	if err != nil {
		t.Fatalf("create carol code: %v", err)
	}
	if err := svc.Redeem(ctx, "bob", aliceCode); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if err := svc.Redeem(ctx, "bob", carolCode); err != referral.ErrAlreadyRedeemed {
		t.Fatalf("expected ErrAlreadyRedeemed redeeming a different code, got %v", err)
	}
	if err := svc.Redeem(ctx, "alice", aliceCode); err != referral.ErrSelfReferral {
		t.Fatalf("expected ErrSelfReferral, got %v", err)
	}

	alice, err := svc.GetMyReferral(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if alice.InvitedCount != 1 || alice.RewardsTotal != referral.RewardOnReferral {
		t.Fatalf("alice should keep exactly one reward, got invited=%d total=%d", alice.InvitedCount, alice.RewardsTotal)
	}
	rewards, _, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("list alice: %v", err)
	}
	if len(rewards) != 1 {
		t.Fatalf("expected exactly one reward for alice, got %d", len(rewards))
	}
	carol, err := svc.GetMyReferral(ctx, "carol")
	if err != nil {
		t.Fatalf("get carol: %v", err)
	}
	if carol.InvitedCount != 0 || carol.RewardsTotal != 0 {
		t.Fatalf("refused redeem must not credit carol, got invited=%d total=%d", carol.InvitedCount, carol.RewardsTotal)
	}
}

func seedRewards(t *testing.T, repo *fake.InMemoryReferralRepo, user string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := repo.AddReward(context.Background(), user, 1, "seed"); err != nil {
			t.Fatalf("seed reward: %v", err)
		}
	}
}

func TestListRewardsPageSizeClamp(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	svc := referral.NewService(repo)
	ctx := context.Background()
	seedRewards(t, repo, "alice", 150)

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
	repo := fake.NewInMemoryReferralRepo()
	svc := referral.NewService(repo)
	ctx := context.Background()
	seedRewards(t, repo, "alice", 5)

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
