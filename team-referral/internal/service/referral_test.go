package service_test

import (
	"context"
	"testing"

	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/repository"
	"github.com/buidangphuc/team-referral/internal/service"
)

func newReferralService() *service.ReferralService {
	return service.NewReferralService(repository.NewInMemoryReferralRepo())
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
	if alice.RewardsTotal != service.RewardOnReferral {
		t.Fatalf("expected alice rewards=%d, got %d", service.RewardOnReferral, alice.RewardsTotal)
	}

	rewards, _, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("list rewards: %v", err)
	}
	if len(rewards) != 1 || rewards[0].GetReason() != service.ReasonInviteeJoined {
		t.Fatalf("expected one %q reward, got %+v", service.ReasonInviteeJoined, rewards)
	}

	// A double redemption by the same referee is rejected.
	if err := svc.Redeem(ctx, "bob", code); err != service.ErrAlreadyRedeemed {
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
	if err := svc.Redeem(ctx, "alice", code); err != service.ErrSelfReferral {
		t.Fatalf("expected ErrSelfReferral, got %v", err)
	}
	if err := svc.Redeem(ctx, "bob", "REFnope123"); err != service.ErrCodeNotFound {
		t.Fatalf("expected ErrCodeNotFound for unknown code, got %v", err)
	}
	if err := svc.Redeem(ctx, "bob", ""); err != service.ErrEmptyCode {
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

	if _, err := svc.CreateCode(ctx, ""); err != service.ErrEmptyUser {
		t.Fatalf("CreateCode empty user: got %v", err)
	}
	if _, err := svc.GetMyReferral(ctx, ""); err != service.ErrEmptyUser {
		t.Fatalf("GetMyReferral empty user: got %v", err)
	}
	if err := svc.Redeem(ctx, "", "REFx"); err != service.ErrEmptyUser {
		t.Fatalf("Redeem empty referee: got %v", err)
	}
	if _, _, err := svc.ListRewards(ctx, "", nil); err != service.ErrEmptyUser {
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
	// Three redemptions → three reward rows on alice's ledger.
	for _, referee := range []string{"u1", "u2", "u3"} {
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
	if cursor == "" {
		t.Fatal("expected a next-page cursor after page 1")
	}

	page2, cursor2, err := svc.ListRewards(ctx, "alice", &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2, Cursor: cursor},
	})
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 reward on page 2, got %d", len(page2))
	}
	if cursor2 != "" {
		t.Fatalf("expected no cursor after the last page, got %q", cursor2)
	}
}
