package referral_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store"
)

func assertNothingWritten(t *testing.T, f *fixture, referee string) {
	t.Helper()
	if _, err := f.repo.GetReferral(context.Background(), referee); !errors.Is(err, store.ErrReferralNotFound) {
		t.Fatalf("GetReferral(%s) = %v, want ErrReferralNotFound", referee, err)
	}
	if n := len(f.repo.OutboxRows()); n != 0 {
		t.Fatalf("%d outbox rows written by a refused redeem", n)
	}
}

func aliceWithCode(t *testing.T, f *fixture) string {
	t.Helper()
	code, err := f.svc.CreateCode(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestRedeemRefusesOldAccount(t *testing.T) {
	f := newFixture()
	code := aliceWithCode(t, f)
	f.register(t, "dave", t0.Add(-169*time.Hour))

	if err := f.svc.Redeem(context.Background(), "dave", code); !errors.Is(err, referral.ErrNotEligible) {
		t.Fatalf("Redeem = %v, want ErrNotEligible", err)
	}
	assertNothingWritten(t, f, "dave")
}

func TestRedeemAgeBoundary(t *testing.T) {
	f := newFixture()
	code := aliceWithCode(t, f)
	f.register(t, "at-limit", t0.Add(-168*time.Hour))
	f.register(t, "just-under", t0.Add(-(168*time.Hour - time.Minute)))

	if err := f.svc.Redeem(context.Background(), "at-limit", code); !errors.Is(err, referral.ErrNotEligible) {
		t.Fatalf("at the limit: Redeem = %v, want ErrNotEligible", err)
	}
	assertNothingWritten(t, f, "at-limit")
	if err := f.svc.Redeem(context.Background(), "just-under", code); err != nil {
		t.Fatalf("just under the limit: Redeem = %v, want success", err)
	}
	if ref, err := f.repo.GetReferral(context.Background(), "just-under"); err != nil || ref.Status != store.StatusAttributed {
		t.Fatalf("referral = %+v, %v", ref, err)
	}
}

func TestRedeemRefusesAfterCompletedOrder(t *testing.T) {
	f := newFixture()
	code := aliceWithCode(t, f)
	f.register(t, "erin", t0.Add(-time.Hour))
	err := f.repo.ApplyOrderCompleted(context.Background(), store.OrderCompletedInput{
		EventID: "oc-erin", OrderID: "o-erin", BuyerID: "erin", CompletedAt: t0.Add(-time.Minute),
		ReleaseAt: t0.Add(time.Hour), At: t0.Add(-time.Minute), RewardXU: 10000, LifetimeCap: 50,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.svc.Redeem(context.Background(), "erin", code); !errors.Is(err, referral.ErrNotEligible) {
		t.Fatalf("Redeem = %v, want ErrNotEligible", err)
	}
	assertNothingWritten(t, f, "erin")
}

func TestRedeemRefusesUnknownRegistration(t *testing.T) {
	f := newFixture()
	code := aliceWithCode(t, f)

	if err := f.svc.Redeem(context.Background(), "legacy-user", code); !errors.Is(err, referral.ErrNotEligible) {
		t.Fatalf("Redeem = %v, want ErrNotEligible", err)
	}
	assertNothingWritten(t, f, "legacy-user")
}

func TestRedeemRefusesCappedReferrer(t *testing.T) {
	f := newFixture()
	code := f.inviterWithRewards(t, "alice", 50, t0.Add(48*time.Hour))
	f.registerNow(t, "bob")

	if err := f.svc.Redeem(context.Background(), "bob", code); !errors.Is(err, referral.ErrReferrerCapped) {
		t.Fatalf("Redeem = %v, want ErrReferrerCapped", err)
	}
	if _, err := f.repo.GetReferral(context.Background(), "bob"); !errors.Is(err, store.ErrReferralNotFound) {
		t.Fatalf("a referral row exists for bob: %v", err)
	}
}
