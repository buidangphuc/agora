package rpc_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/rpc"
	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

func TestSummaryScopedToPrincipal(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	code, err := repo.CreateCode(context.Background(), "alice", "REFAAAA1111")
	if err != nil {
		t.Fatal(err)
	}
	seedReward(t, repo, "alice", code, "ref1", "o1", time.Now().Add(48*time.Hour))
	seedReward(t, repo, "alice", code, "ref2", "o2", time.Now().Add(-time.Hour))
	if _, err := referral.NewSweeper(repo).Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	mine, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if mine.GetPendingXu() != 10000 || mine.GetReleasedXu() != 10000 || mine.GetRewardsTotal() != 10000 {
		t.Fatalf("alice summary = %v", mine)
	}

	bob, err := h.GetMyReferral(userCtx("bob"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if bob.GetPendingXu() != 0 || bob.GetReleasedXu() != 0 || bob.GetRewardsTotal() != 0 {
		t.Fatalf("bob sees alice's totals: %v", bob)
	}
	list, err := h.ListReferralRewards(userCtx("bob"), &referralv1.ListReferralRewardsRequest{})
	if err != nil || len(list.GetRewards()) != 0 {
		t.Fatalf("bob list = %v, %v", list, err)
	}
}

func TestRedeemRefusalsAreFailedPreconditionWithVietnameseMessages(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	registerNow(t, repo, "alice", "bob", "carol")
	if _, err := repo.CreateCode(context.Background(), "carol", "REFCCCC2222"); err != nil {
		t.Fatal(err)
	}
	created, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: created.GetCode()}); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	cases := map[string]error{}
	_, cases["self"] = h.RedeemReferral(userCtx("alice"), &referralv1.RedeemReferralRequest{Code: created.GetCode()})
	_, cases["second"] = h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: "REFCCCC2222"})
	_, cases["unregistered"] = h.RedeemReferral(userCtx("legacy"), &referralv1.RedeemReferralRequest{Code: created.GetCode()})
	for name, err := range cases {
		st, _ := status.FromError(err)
		if st.Code() != codes.FailedPrecondition {
			t.Errorf("%s: code = %v, want FailedPrecondition", name, st.Code())
		}
		if !hasVietnamese(st.Message()) {
			t.Errorf("%s: message %q is not Vietnamese", name, st.Message())
		}
	}

	capped := fake.NewInMemoryReferralRepo()
	hc := handlerFor(t, capped)
	code, _ := capped.CreateCode(context.Background(), "owner", "REFOWNER001")
	for i := 0; i < 50; i++ {
		seedReward(t, capped, "owner", code, "r"+string(rune('a'+i%26))+string(rune('a'+i/26)), "ord"+string(rune('a'+i%26))+string(rune('a'+i/26)), time.Now().Add(48*time.Hour))
	}
	registerNow(t, capped, "late")
	_, err = hc.RedeemReferral(userCtx("late"), &referralv1.RedeemReferralRequest{Code: code})
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition || !hasVietnamese(st.Message()) {
		t.Errorf("capped owner: %v %q", st.Code(), st.Message())
	}
}

func hasVietnamese(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}

func TestRPCLogsCarryNoRawIdentifiers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	repo := newFaulty()
	repo.redeemErr = context.DeadlineExceeded
	svc := referral.NewService(repo)
	h, err := rpc.NewHandler(svc, logger)
	if err != nil {
		t.Fatal(err)
	}
	registerNow(t, repo, "user-zed-5e91c0")
	code, _ := repo.CreateCode(context.Background(), "user-owner-77ab", "REFRAWCODE1")
	_, _ = h.RedeemReferral(userCtx("user-zed-5e91c0"), &referralv1.RedeemReferralRequest{Code: code})
	repo.redeemErr = store.ErrCodeNotFound
	_, _ = h.RedeemReferral(userCtx("user-zed-5e91c0"), &referralv1.RedeemReferralRequest{Code: code})

	for _, raw := range []string{"user-zed-5e91c0", "user-owner-77ab", "REFRAWCODE1"} {
		if strings.Contains(buf.String(), raw) {
			t.Errorf("rpc logs contain %q: %s", raw, buf.String())
		}
	}
}
