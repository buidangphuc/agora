package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-referral/internal/store"
)

var windowEnd = t0.Add(7 * 24 * time.Hour)

func invitedPair(t *testing.T, e *env) {
	t.Helper()
	code := aliceCode(t, e)
	e.mustRegister(t, "bob", code)
}

func TestOrderCompletedCreatesPendingReward(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)

	e.mustComplete(t, "o1", "bob", windowEnd)

	rw, err := e.repo.GetRewardByOrder(context.Background(), "o1")
	if err != nil {
		t.Fatalf("no reward for o1: %v", err)
	}
	if rw.ReferrerID != "alice" || rw.Status != store.RewardPending || rw.AmountXU != 10000 || rw.Reason != "invitee_first_order" || !rw.ReleaseAt.Equal(windowEnd) {
		t.Fatalf("reward = %+v", rw)
	}
	ref, _ := e.repo.GetReferral(context.Background(), "bob")
	if ref.Status != store.StatusQualified {
		t.Fatalf("referral = %s, want QUALIFIED", ref.Status)
	}
	tot, err := e.repo.RewardTotals(context.Background(), "alice")
	if err != nil || tot.PendingXU != 10000 || tot.ReleasedXU != 0 {
		t.Fatalf("totals = %+v, %v", tot, err)
	}
}

func TestOrderCompletedWithoutReferral(t *testing.T) {
	e := newEnv(t)
	e.mustComplete(t, "o1", "dave", windowEnd)

	if _, err := e.repo.GetRewardByOrder(context.Background(), "o1"); err == nil {
		t.Fatal("a reward was created for a buyer with no referral")
	}
	if n := len(e.rows()); n != 0 {
		t.Fatalf("%d outbox rows, want 0", n)
	}
	if has, err := e.repo.HasCompletedOrder(context.Background(), "dave"); err != nil || !has {
		t.Fatalf("HasCompletedOrder(dave) = %v, %v; the completed-order fact must be stored", has, err)
	}
}

func TestSecondOrderDoesNotCreateSecondReward(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)
	e.mustComplete(t, "o1", "bob", windowEnd)
	e.mustComplete(t, "o2", "bob", windowEnd)

	if _, err := e.repo.GetRewardByOrder(context.Background(), "o2"); err == nil {
		t.Fatal("second order created a reward")
	}
	tot, _ := e.repo.RewardTotals(context.Background(), "alice")
	if tot.PendingXU != 10000 {
		t.Fatalf("pending = %d, want exactly one reward (10000)", tot.PendingXU)
	}
}

func TestOrderCompletedRedelivery(t *testing.T) {
	t.Run("same event id", func(t *testing.T) {
		e := newEnv(t)
		invitedPair(t, e)
		for i := 0; i < 2; i++ {
			if err := e.completed(t, "oc-1", "o1", "bob", windowEnd); err != nil {
				t.Fatal(err)
			}
		}
		if tot, _ := e.repo.RewardTotals(context.Background(), "alice"); tot.PendingXU != 10000 {
			t.Fatalf("pending = %d, want 10000", tot.PendingXU)
		}
	})
	t.Run("different event id same order", func(t *testing.T) {
		e := newEnv(t)
		invitedPair(t, e)
		if err := e.completed(t, "oc-1", "o1", "bob", windowEnd); err != nil {
			t.Fatal(err)
		}
		if err := e.completed(t, "oc-2", "o1", "bob", windowEnd); err != nil {
			t.Fatal(err)
		}
		if tot, _ := e.repo.RewardTotals(context.Background(), "alice"); tot.PendingXU != 10000 {
			t.Fatalf("pending = %d, want 10000", tot.PendingXU)
		}
	})
}

func TestOrderCompletedAtLifetimeCap(t *testing.T) {
	e := newEnv(t)
	e.seedRewards(t, "alice", 49)
	code, _ := e.repo.GetCodeByUser(context.Background(), "alice")
	e.mustRegister(t, "bob", code)
	e.mustRegister(t, "zed", code)
	e.mustComplete(t, "o-zed", "zed", windowEnd)

	e.mustComplete(t, "o-late", "bob", windowEnd)

	if _, err := e.repo.GetRewardByOrder(context.Background(), "o-late"); err == nil {
		t.Fatal("reward created beyond the lifetime cap")
	}
	ref, _ := e.repo.GetReferral(context.Background(), "bob")
	if ref.Status != store.StatusCapped {
		t.Fatalf("referral = %s, want CAPPED", ref.Status)
	}
}

func TestReturnRequestedVoidsPendingReward(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)
	e.mustComplete(t, "o1", "bob", windowEnd)

	if err := e.returned(t, "rr-1", "o1"); err != nil {
		t.Fatal(err)
	}
	rw, _ := e.repo.GetRewardByOrder(context.Background(), "o1")
	ref, _ := e.repo.GetReferral(context.Background(), "bob")
	tot, _ := e.repo.RewardTotals(context.Background(), "alice")
	if rw.Status != store.RewardVoid || ref.Status != store.StatusAttributed || tot.PendingXU != 0 {
		t.Fatalf("reward=%s referral=%s pending=%d", rw.Status, ref.Status, tot.PendingXU)
	}
}

func TestRequalifyAfterVoid(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)
	e.mustComplete(t, "o1", "bob", windowEnd)
	if err := e.returned(t, "rr-1", "o1"); err != nil {
		t.Fatal(err)
	}

	e.mustComplete(t, "o3", "bob", windowEnd)

	rw, err := e.repo.GetRewardByOrder(context.Background(), "o3")
	if err != nil || rw.Status != store.RewardPending {
		t.Fatalf("reward for o3 = %+v, %v", rw, err)
	}
	if ref, _ := e.repo.GetReferral(context.Background(), "bob"); ref.Status != store.StatusQualified {
		t.Fatalf("referral = %s, want QUALIFIED", ref.Status)
	}
}

func TestReturnAfterReleaseIgnored(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)
	e.mustComplete(t, "o1", "bob", t0.Add(time.Hour))
	n, err := e.repo.ReleaseDue(context.Background(), store.ReleaseInput{At: t0.Add(2 * time.Hour), DailyCap: 5, LifetimeCap: 50, Location: time.UTC})
	if err != nil || n != 1 {
		t.Fatalf("release = %d, %v", n, err)
	}

	if err := e.returned(t, "rr-1", "o1"); err != nil {
		t.Fatal(err)
	}
	rw, _ := e.repo.GetRewardByOrder(context.Background(), "o1")
	tot, _ := e.repo.RewardTotals(context.Background(), "alice")
	if rw.Status != store.RewardReleased || tot.ReleasedXU != 10000 {
		t.Fatalf("reward=%s released=%d", rw.Status, tot.ReleasedXU)
	}
}

func TestReturnRequestedUnknownOrder(t *testing.T) {
	e := newEnv(t)
	invitedPair(t, e)
	e.mustComplete(t, "o1", "bob", windowEnd)
	before := len(e.rows())

	if err := e.returned(t, "rr-9", "o-unrelated"); err != nil {
		t.Fatal(err)
	}
	if rw, _ := e.repo.GetRewardByOrder(context.Background(), "o1"); rw.Status != store.RewardPending {
		t.Fatalf("unrelated return changed the reward: %s", rw.Status)
	}
	if n := len(e.rows()); n != before {
		t.Fatalf("outbox changed from %d to %d", before, n)
	}
}
