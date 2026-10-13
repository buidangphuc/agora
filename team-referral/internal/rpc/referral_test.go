package rpc_test

import (
	"context"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/rpc"
	"github.com/buidangphuc/team-referral/internal/store"
)

func newHandler(t testing.TB) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(referral.NewService(store.NewInMemoryReferralRepo()), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id})
}

// Anonymous callers (no principal) are rejected on every user-owned RPC.
func TestHandlerRejectsAnonymous(t *testing.T) {
	h := newHandler(t)
	anon := context.Background() // no interceptor ran → anonymous

	if _, err := h.CreateReferralCode(anon, &referralv1.CreateReferralCodeRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateReferralCode: expected Unauthenticated, got %v", err)
	}
	if _, err := h.GetMyReferral(anon, &referralv1.GetMyReferralRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetMyReferral: expected Unauthenticated, got %v", err)
	}
	if _, err := h.RedeemReferral(anon, &referralv1.RedeemReferralRequest{Code: "REFx"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("RedeemReferral: expected Unauthenticated, got %v", err)
	}
	if _, err := h.ListReferralRewards(anon, &referralv1.ListReferralRewardsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListReferralRewards: expected Unauthenticated, got %v", err)
	}
}

// End-to-end through the handler: the caller is scoped from the principal, and a
// redeem by another principal attributes the referee to the code owner.
func TestHandlerScopesToPrincipal(t *testing.T) {
	h := newHandler(t)

	created, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	code := created.GetCode()

	if _, err := h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: code}); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	mine, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if mine.GetInvitedCount() != 1 || mine.GetRewardsTotal() != referral.RewardOnReferral {
		t.Fatalf("expected invited=1 total=%d, got invited=%d total=%d",
			referral.RewardOnReferral, mine.GetInvitedCount(), mine.GetRewardsTotal())
	}

	// Redeeming a missing code surfaces NotFound to the caller.
	if _, err := h.RedeemReferral(userCtx("carol"), &referralv1.RedeemReferralRequest{Code: "REFmissing"}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for unknown code, got %v", err)
	}
}
