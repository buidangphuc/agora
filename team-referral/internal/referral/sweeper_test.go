package referral_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store"
)

const typeReleased = "platform.referral.v1.ReferralRewardReleased"

func (f *fixture) releasedRows() int {
	n := 0
	for _, r := range f.repo.OutboxRows() {
		if r.EventType == typeReleased {
			n++
		}
	}
	return n
}

func (f *fixture) sweep(t *testing.T, at time.Time) int {
	t.Helper()
	f.clock.now = at
	n, err := f.sweeper().Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep at %v: %v", at, err)
	}
	return n
}

func (f *fixture) status(t *testing.T, order string) string {
	t.Helper()
	rw, err := f.repo.GetRewardByOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("reward for %s: %v", order, err)
	}
	return rw.Status
}

func TestSweepReleasesDueReward(t *testing.T) {
	f := newFixture()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	f.invitee(t, "alice", code, "bob", "o1", t0.Add(-10*time.Second))

	if n := f.sweep(t, t0); n != 1 {
		t.Fatalf("Sweep released %d, want 1", n)
	}
	rw, _ := f.repo.GetRewardByOrder(context.Background(), "o1")
	if rw.Status != store.RewardReleased || rw.ReleasedAt == nil || !rw.ReleasedAt.Equal(t0) {
		t.Fatalf("reward = %+v", rw)
	}
	if ref, _ := f.repo.GetReferral(context.Background(), "bob"); ref.Status != store.StatusRewarded {
		t.Fatalf("referral = %s, want REWARDED", ref.Status)
	}
	my, _ := f.svc.GetMyReferral(context.Background(), "alice")
	if my.ReleasedXU != 10000 || my.PendingXU != 0 {
		t.Fatalf("summary = %+v", my)
	}
	var rows int
	for _, r := range f.repo.OutboxRows() {
		if r.EventType != typeReleased {
			continue
		}
		rows++
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Payload, &env); err != nil {
			t.Fatal(err)
		}
		var p referralv1.ReferralRewardReleased
		if err := proto.Unmarshal(env.GetPayload(), &p); err != nil {
			t.Fatal(err)
		}
		if p.GetAmountXu() != 10000 || p.GetRewardId() != rw.ID || p.GetReferrerId() != "alice" || p.GetRefereeId() != "bob" || r.AggregateID != "bob" {
			t.Fatalf("released event = %v on aggregate %q", &p, r.AggregateID)
		}
	}
	if rows != 1 {
		t.Fatalf("%d ReferralRewardReleased rows, want 1", rows)
	}
	if n := f.sweep(t, t0.Add(time.Minute)); n != 0 || f.releasedRows() != 1 {
		t.Fatalf("second sweep released %d and rows=%d; a reward is released once", n, f.releasedRows())
	}
}

func TestSweepKeepsFutureReward(t *testing.T) {
	f := newFixture()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	f.invitee(t, "alice", code, "bob", "o1", t0.Add(time.Hour))
	before := len(f.repo.OutboxRows())

	if n := f.sweep(t, t0); n != 0 {
		t.Fatalf("Sweep released %d, want 0", n)
	}
	if got := f.status(t, "o1"); got != store.RewardPending {
		t.Fatalf("reward = %s, want PENDING", got)
	}
	if n := len(f.repo.OutboxRows()); n != before {
		t.Fatalf("outbox grew from %d to %d", before, n)
	}
}

func sixDue(t *testing.T, f *fixture, first, step time.Duration) {
	t.Helper()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	for i := 1; i <= 6; i++ {
		f.invitee(t, "alice", code, fmt.Sprintf("ref%d", i), fmt.Sprintf("o%d", i), t0.Add(first+time.Duration(i)*step))
	}
}

func TestSweepDailyCapDefersToNextDay(t *testing.T) {
	f := newFixture()
	sixDue(t, f, -7*time.Hour, time.Hour)

	if n := f.sweep(t, t0); n != 5 {
		t.Fatalf("first sweep released %d, want 5", n)
	}
	for i := 1; i <= 5; i++ {
		if got := f.status(t, fmt.Sprintf("o%d", i)); got != store.RewardReleased {
			t.Fatalf("o%d = %s, want RELEASED (oldest release_at first)", i, got)
		}
	}
	if got := f.status(t, "o6"); got != store.RewardPending {
		t.Fatalf("o6 = %s, want PENDING", got)
	}
	if n := f.sweep(t, t0.Add(time.Hour)); n != 0 {
		t.Fatalf("same-day retry released %d, want 0", n)
	}

	nextDay := time.Date(2026, 11, 6, 17, 30, 0, 0, time.UTC)
	if n := f.sweep(t, nextDay); n != 1 {
		t.Fatalf("next-day sweep released %d, want 1", n)
	}
	if got := f.status(t, "o6"); got != store.RewardReleased {
		t.Fatalf("o6 = %s, want RELEASED", got)
	}
}

func TestDailyCapUsesConfiguredTimezone(t *testing.T) {
	setup := func(f *fixture) {
		code, _ := f.svc.CreateCode(context.Background(), "alice")
		due := time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC)
		for i := 1; i <= 5; i++ {
			f.invitee(t, "alice", code, fmt.Sprintf("ref%d", i), fmt.Sprintf("o%d", i), due)
		}
		f.invitee(t, "alice", code, "ref6", "o6", time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC))
	}
	before := time.Date(2026, 11, 6, 16, 59, 0, 0, time.UTC)
	after := time.Date(2026, 11, 6, 17, 1, 0, 0, time.UTC)

	t.Run("Asia/Ho_Chi_Minh", func(t *testing.T) {
		f := newFixture()
		setup(f)
		if n := f.sweep(t, before); n != 5 {
			t.Fatalf("23:59 sweep released %d, want 5", n)
		}
		if n := f.sweep(t, after); n != 1 {
			t.Fatalf("00:01 sweep released %d, want 1 (new calendar day in Asia/Ho_Chi_Minh)", n)
		}
	})
	t.Run("UTC", func(t *testing.T) {
		f := newFixture(func(c *referral.Config) { c.CapLocation = time.UTC })
		setup(f)
		if n := f.sweep(t, before); n != 5 {
			t.Fatalf("16:59 sweep released %d, want 5", n)
		}
		if n := f.sweep(t, after); n != 0 {
			t.Fatalf("17:01 UTC sweep released %d, want 0 (same UTC day)", n)
		}
	})
}

func TestSweepLifetimeCap(t *testing.T) {
	f := newFixture()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	for i := 1; i <= 3; i++ {
		f.invitee(t, "alice", code, fmt.Sprintf("ref%d", i), fmt.Sprintf("o%d", i), t0.Add(-time.Hour))
	}
	f.invitee(t, "alice", code, "ref4", "o4", t0.Add(time.Hour))
	if n := f.sweep(t, t0); n != 3 {
		t.Fatalf("setup sweep released %d, want 3", n)
	}

	f.cfg.LifetimeCap = 3
	if n := f.sweep(t, t0.Add(2*time.Hour)); n != 0 {
		t.Fatalf("sweep with lifetime cap 3 released %d, want 0", n)
	}
	if got := f.status(t, "o4"); got != store.RewardPending {
		t.Fatalf("o4 = %s, want PENDING", got)
	}
}

func TestSummarySplitsPendingAndReleased(t *testing.T) {
	f := newFixture()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	f.invitee(t, "alice", code, "ref1", "o1", t0.Add(-2*time.Hour))
	f.invitee(t, "alice", code, "ref2", "o2", t0.Add(-time.Hour))
	f.invitee(t, "alice", code, "ref3", "o3", t0.Add(48*time.Hour))
	f.sweep(t, t0)

	my, err := f.svc.GetMyReferral(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if my.PendingXU != 10000 || my.ReleasedXU != 20000 || my.RewardsTotal != 20000 {
		t.Fatalf("summary = %+v, want pending 10000 released 20000 total 20000", my)
	}
}

func TestListRewardsOmitsVoid(t *testing.T) {
	f := newFixture()
	code, _ := f.svc.CreateCode(context.Background(), "alice")
	f.invitee(t, "alice", code, "ref-void", "o-void", t0.Add(48*time.Hour))
	f.invitee(t, "alice", code, "ref-pending", "o-pending", t0.Add(48*time.Hour))
	f.invitee(t, "alice", code, "ref-released", "o-released", t0.Add(-time.Hour))
	if err := f.repo.ApplyReturnRequested(context.Background(), store.ReturnRequestedInput{EventID: "rr1", OrderID: "o-void", At: t0}); err != nil {
		t.Fatal(err)
	}
	f.sweep(t, t0)

	rows, _, err := f.svc.ListRewards(context.Background(), "alice", &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[referralv1.ReferralRewardStatus]int{}
	for _, r := range rows {
		got[r.GetStatus()]++
	}
	if len(rows) != 2 || got[referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_PENDING] != 1 || got[referralv1.ReferralRewardStatus_REFERRAL_REWARD_STATUS_RELEASED] != 1 {
		t.Fatalf("rows = %v, want one PENDING and one RELEASED and no VOID", rows)
	}
	for _, r := range rows {
		if r.GetAmount() != 10000 || r.GetReleaseAt() == nil {
			t.Fatalf("row %v lacks amount or release_at", r)
		}
	}
}

func TestSweepPurgesProcessedEventsOlderThanSevenDays(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	for id, age := range map[string]time.Duration{"old": 8 * 24 * time.Hour, "fresh": 6 * 24 * time.Hour} {
		err := f.repo.ApplyUserRegistered(ctx, store.UserRegisteredInput{EventID: id, UserID: "u-" + id, RegisteredAt: t0.Add(-age), At: t0.Add(-age), LifetimeCap: 50})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.sweep(t, t0)

	for _, id := range []string{"old", "fresh"} {
		err := f.repo.ApplyUserRegistered(ctx, store.UserRegisteredInput{EventID: id, UserID: "u-" + id, RegisteredAt: t0, At: t0, LifetimeCap: 50})
		if err != nil {
			t.Fatal(err)
		}
	}
	if at, _ := f.repo.RegisteredAt(ctx, "u-old"); !at.Equal(t0) {
		t.Fatalf("old event was still deduplicated after the purge: registered_at = %v", at)
	}
	if at, _ := f.repo.RegisteredAt(ctx, "u-fresh"); at.Equal(t0) {
		t.Fatalf("fresh event was purged: registered_at moved to %v", at)
	}
}
