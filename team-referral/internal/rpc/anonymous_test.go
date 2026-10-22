package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

// Scenario: The gateway's anonymous principal is rejected.
func TestGatewayAnonymousIsUnauthenticated(t *testing.T) {
	cases := []struct {
		name string
		md   metadata.MD
	}{
		{"anonymous id and type", metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous")},
		{"anonymous type with an id", metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "anonymous")},
		{"anonymous id without a type", metadata.Pairs("x-principal-id", "anonymous")},
		{"no metadata", metadata.MD{}},
	}
	for _, c := range cases {
		repo := fake.NewInMemoryReferralRepo()
		client := dial(t, handlerFor(t, repo))
		ctx := metadata.NewOutgoingContext(context.Background(), c.md)

		if _, err := client.CreateReferralCode(ctx, &referralv1.CreateReferralCodeRequest{}); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: CreateReferralCode: %v", c.name, err)
		}
		if _, err := client.GetMyReferral(ctx, &referralv1.GetMyReferralRequest{}); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: GetMyReferral: %v", c.name, err)
		}
		if _, err := client.RedeemReferral(ctx, &referralv1.RedeemReferralRequest{Code: "REFx"}); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: RedeemReferral: %v", c.name, err)
		}
		if _, err := client.ListReferralRewards(ctx, &referralv1.ListReferralRewardsRequest{}); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: ListReferralRewards: %v", c.name, err)
		}
		if _, err := repo.GetCodeByUser(context.Background(), "u-1"); err == nil {
			t.Errorf("%s: a code row was written", c.name)
		}
		if _, err := repo.GetCodeByUser(context.Background(), "anonymous"); err == nil {
			t.Errorf("%s: a code row was written for anonymous", c.name)
		}
	}
}

// Scenario: A call without a principal is rejected, a forwarded user principal is accepted.
func TestForwardedUserPrincipalIsAccepted(t *testing.T) {
	client := dial(t, newHandler(t))
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-principal-id", "u-1", "x-principal-type", "user")
	resp, err := client.CreateReferralCode(ctx, &referralv1.CreateReferralCodeRequest{})
	if err != nil || resp.GetCode() == "" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}
