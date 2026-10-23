package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
)

var badTypes = map[string]string{"typeless": "", "unrecognised": "robot"}

func attempts(h interface {
	SubmitKyc(context.Context, *verificationv1.SubmitKycRequest) (*verificationv1.SubmitKycResponse, error)
	ReviewKyc(context.Context, *verificationv1.ReviewKycRequest) (*verificationv1.ReviewKycResponse, error)
	GetVerificationStatus(context.Context, *verificationv1.GetVerificationStatusRequest) (*verificationv1.GetVerificationStatusResponse, error)
}, ctx context.Context, id string) map[string]error {
	_, s := h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
	_, r := h.ReviewKyc(ctx, &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	_, g := h.GetVerificationStatus(ctx, &verificationv1.GetVerificationStatusRequest{UserId: "alice"})
	return map[string]error{"SubmitKyc": s, "ReviewKyc": r, "GetVerificationStatus other user": g}
}

// Scenario: A principal with no type is refused.
func TestTypelessPrincipalUnauthenticated(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("alice"))
	for name, err := range attempts(e.h, ctxAs("typeless-1", "", "admin,listing.read"), id) {
		if err == nil {
			t.Fatalf("%s: typeless principal with admin scope was accepted", name)
		}
		wantCode(t, err, codes.Unauthenticated)
	}
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertPending(t, id)
}

// Scenario: A principal with an unrecognised type is refused.
func TestUnknownTypePrincipalUnauthenticated(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("alice"))
	for name, err := range attempts(e.h, ctxAs("robot-1", "robot", "admin,listing.read"), id) {
		if err == nil {
			t.Fatalf("%s: robot principal with admin scope was accepted", name)
		}
		wantCode(t, err, codes.Unauthenticated)
	}
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertPending(t, id)
}

// Scenario: Service principal cannot read own status.
func TestStatusServicePrincipalDenied(t *testing.T) {
	h := newHandler()
	for name, req := range map[string]*verificationv1.GetVerificationStatusRequest{
		"empty user_id": {},
		"own user_id":   {UserId: "svc"},
	} {
		_, err := h.GetVerificationStatus(ctxAs("svc", "service", "listing.read"), req)
		if status := err; status == nil {
			t.Fatalf("%s: service principal read a status", name)
		}
		wantCode(t, err, codes.PermissionDenied)
	}
}

// Scenario: Non-admin cannot read another user's status.
func TestStatusOtherUserNeedsAdmin(t *testing.T) {
	h := newHandler()
	submit(t, h, buyer("alice"))
	_, err := h.GetVerificationStatus(buyer("bob"), &verificationv1.GetVerificationStatusRequest{UserId: "alice"})
	wantCode(t, err, codes.PermissionDenied)
}

// Scenario: Admin can read another user's status.
func TestStatusAdminReadsOtherUser(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	if _, err := h.ReviewKyc(admin("adm-1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"}); err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	st, err := h.GetVerificationStatus(admin("adm-1"), &verificationv1.GetVerificationStatusRequest{UserId: "alice"})
	if err != nil || !st.GetBadge() || st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("admin should read alice VERIFIED, got %v err=%v", st, err)
	}
}

// Scenario: Anonymous caller cannot read a status.
func TestStatusAnonymousUnauthenticated(t *testing.T) {
	h := newHandler()
	for name, ctx := range map[string]context.Context{"anonymous": anonymous(), "no principal": context.Background()} {
		for rname, req := range map[string]*verificationv1.GetVerificationStatusRequest{"own": {}, "other": {UserId: "alice"}} {
			_, err := h.GetVerificationStatus(ctx, req)
			if err == nil {
				t.Fatalf("%s/%s: anonymous read a status", name, rname)
			}
			wantCode(t, err, codes.Unauthenticated)
		}
	}
}
