package edge_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
)

func deletionCalls(f *privacyFixture, auth string) map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"RequestAccountDeletion": func() error {
			_, err := f.client.RequestAccountDeletion(ctx, privacyReq(&identityv1.RequestAccountDeletionRequest{}, auth))
			return err
		},
		"CancelAccountDeletion": func() error {
			_, err := f.client.CancelAccountDeletion(ctx, privacyReq(&identityv1.CancelAccountDeletionRequest{}, auth))
			return err
		},
		"GetAccountDeletionStatus": func() error {
			_, err := f.client.GetAccountDeletionStatus(ctx, privacyReq(&identityv1.GetAccountDeletionStatusRequest{}, auth))
			return err
		},
	}
}

// Scenario: identity.account-deletion-signed-in-user-reaches-the-rpc
func TestIdentityAccountDeletionSignedInUserReachesTheRpc(t *testing.T) {
	f := newPrivacyFixture(t)
	for name, call := range deletionCalls(f, f.userBearer(t)) {
		before := f.spy.otherCalls.Load()
		if err := call(); err != nil {
			t.Fatalf("%s signed in: %v", name, err)
		}
		if got := f.spy.otherCalls.Load(); got != before+1 {
			t.Fatalf("%s: identity calls = %d, want %d", name, got, before+1)
		}
	}
}

// Scenario: identity.account-deletion-anonymous-request-is-rejected-at-the-edge
func TestIdentityAccountDeletionAnonymousRequestIsRejectedAtTheEdge(t *testing.T) {
	f := newPrivacyFixture(t)
	for name, call := range deletionCalls(f, "") {
		if got := connect.CodeOf(call()); got != connect.CodeUnauthenticated {
			t.Errorf("%s anonymous = %v, want unauthenticated", name, got)
		}
	}
	if n := f.spy.otherCalls.Load(); n != 0 {
		t.Fatalf("identity calls for anonymous deletion RPCs = %d, want 0", n)
	}
}

func TestIdentityAccountDeletionRequestForwardsSessionID(t *testing.T) {
	f := newPrivacyFixture(t)
	auth := f.userBearerWithSID(t, "sid-verified")
	for name, call := range deletionCalls(f, auth) {
		if err := call(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got := f.spy.mdOf("RequestAccountDeletion").Get("x-principal-sid"); len(got) != 1 || got[0] != "sid-verified" {
		t.Fatalf("RequestAccountDeletion x-principal-sid = %v, want [sid-verified]", got)
	}
	for _, name := range []string{"CancelAccountDeletion", "GetAccountDeletionStatus"} {
		if got := f.spy.mdOf(name).Get("x-principal-sid"); len(got) != 0 {
			t.Fatalf("%s x-principal-sid = %v, want none", name, got)
		}
	}
}

func TestIdentityAccountDeletionRequestDropsClientSessionID(t *testing.T) {
	f := newPrivacyFixture(t)
	req := privacyReq(&identityv1.RequestAccountDeletionRequest{}, f.userBearer(t))
	req.Header().Set("X-Principal-Sid", "sid-forged")
	if _, err := f.client.RequestAccountDeletion(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := f.spy.mdOf("RequestAccountDeletion").Get("x-principal-sid"); len(got) != 0 {
		t.Fatalf("x-principal-sid = %v, want none", got)
	}
}
