package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/handler"
	"github.com/buidangphuc/team-verification/internal/interceptor"
	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
)

func newHandler() *handler.VerificationHandler {
	svc := service.NewVerificationService(repository.NewInMemoryKycRepo())
	return handler.NewVerificationHandler(svc)
}

// ctxAs resolves a principal exactly as the gRPC server does: forwarded
// x-principal-* metadata run through the real unary interceptor.
func ctxAs(id, ptype, scopes string) context.Context {
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", ptype, "x-principal-scopes", scopes)
	in := metadata.NewIncomingContext(context.Background(), md)
	var out context.Context
	_, _ = interceptor.UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) { out = c; return nil, nil })
	return out
}

func buyer(id string) context.Context { return ctxAs(id, "user", "listing.read,search:read") }
func admin(id string) context.Context { return ctxAs(id, "user", "listing.read,admin") }

func anonymous() context.Context { return ctxAs("anonymous", "anonymous", "listing.read") }

func service1() context.Context { return ctxAs("svc", "service", "admin") }

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("expected %v, got %v (%v)", want, got, err)
	}
}

func submit(t *testing.T, h *handler.VerificationHandler, ctx context.Context) string {
	t.Helper()
	sub, err := h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "mock-ref"})
	if err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
	return sub.GetId()
}

// Submit -> PENDING, an admin approves -> VERIFIED + badge for the owner.
func TestHandlerSubmitReviewFlow(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("seller_1"))

	st, err := h.GetVerificationStatus(buyer("seller_1"), &verificationv1.GetVerificationStatusRequest{})
	if err != nil {
		t.Fatalf("GetVerificationStatus: %v", err)
	}
	if st.GetBadge() || st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("expected PENDING/no badge, got %v badge=%v", st.GetStatus(), st.GetBadge())
	}

	rev, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	if err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("expected VERIFIED, got %v", rev.GetStatus())
	}

	// Owner passes their own id explicitly: still their own status.
	st2, err := h.GetVerificationStatus(buyer("seller_1"), &verificationv1.GetVerificationStatusRequest{UserId: "seller_1"})
	if err != nil {
		t.Fatalf("GetVerificationStatus: %v", err)
	}
	if st2.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED || !st2.GetBadge() {
		t.Fatalf("expected VERIFIED/badge, got %v badge=%v", st2.GetStatus(), st2.GetBadge())
	}
}

func TestTwoUsersDoNotSeeEachOthersStatus(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	if _, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"}); err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}

	bob, err := h.GetVerificationStatus(buyer("bob"), &verificationv1.GetVerificationStatusRequest{})
	if err != nil {
		t.Fatalf("bob status: %v", err)
	}
	if bob.GetBadge() {
		t.Fatalf("bob must not inherit alice's verified badge")
	}
	alice, err := h.GetVerificationStatus(buyer("alice"), &verificationv1.GetVerificationStatusRequest{})
	if err != nil || !alice.GetBadge() {
		t.Fatalf("alice should be verified, got %v err=%v", alice, err)
	}
}

func TestAnonymousAndNoPrincipalAreUnauthenticated(t *testing.T) {
	h := newHandler()
	for name, ctx := range map[string]context.Context{"anonymous": anonymous(), "no principal": context.Background()} {
		_, err := h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s submit: expected Unauthenticated, got %v", name, err)
		}
		_, err = h.GetVerificationStatus(ctx, &verificationv1.GetVerificationStatusRequest{})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s status: expected Unauthenticated, got %v", name, err)
		}
		_, err = h.ReviewKyc(ctx, &verificationv1.ReviewKycRequest{Id: "x", Decision: "approve"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s review: expected Unauthenticated, got %v", name, err)
		}
	}
}

func TestServicePrincipalCannotSubmit(t *testing.T) {
	_, err := newHandler().SubmitKyc(service1(), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
	wantCode(t, err, codes.PermissionDenied)
}

func TestBuyerCannotReview(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	// Not even their own submission.
	_, err := h.ReviewKyc(buyer("alice"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	wantCode(t, err, codes.PermissionDenied)
	_, err = h.ReviewKyc(buyer("bob"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	wantCode(t, err, codes.PermissionDenied)
	got, _ := h.GetVerificationStatus(buyer("alice"), &verificationv1.GetVerificationStatusRequest{})
	if got.GetBadge() {
		t.Fatalf("denied review must not change status")
	}
}

func TestAdminCanReviewSomeoneElse(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	rev, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "reject"})
	if err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED {
		t.Fatalf("expected REJECTED, got %v", rev.GetStatus())
	}
}

func TestAdminCannotReviewOwnSubmission(t *testing.T) {
	h := newHandler()
	id := submit(t, h, admin("admin_1"))
	_, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	wantCode(t, err, codes.PermissionDenied)
	st, _ := h.GetVerificationStatus(admin("admin_1"), &verificationv1.GetVerificationStatusRequest{})
	if st.GetBadge() {
		t.Fatalf("self-review must not verify the submitter")
	}
}

func TestStatusForAnotherUser(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	if _, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"}); err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	req := &verificationv1.GetVerificationStatusRequest{UserId: "alice"}

	st, err := h.GetVerificationStatus(admin("admin_1"), req)
	if err != nil || !st.GetBadge() {
		t.Fatalf("admin should read alice's status, got %v err=%v", st, err)
	}
	_, err = h.GetVerificationStatus(buyer("bob"), req)
	wantCode(t, err, codes.PermissionDenied)
}

// Reviewing a non-existent submission surfaces gRPC NotFound.
func TestHandlerReviewMissingIsNotFound(t *testing.T) {
	_, err := newHandler().ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: "kyc_missing", Decision: "approve"})
	wantCode(t, err, codes.NotFound)
}

// An invalid review decision surfaces gRPC InvalidArgument.
func TestHandlerBadDecisionIsInvalidArgument(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("u"))
	_, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "meh"})
	wantCode(t, err, codes.InvalidArgument)
}

// A decision is final: a second review (or a concurrent one that loses) gets
// FailedPrecondition and leaves the first decision in place.
func TestReviewIsFinal(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	if _, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"}); err != nil {
		t.Fatalf("first review: %v", err)
	}
	_, err := h.ReviewKyc(admin("admin_2"), &verificationv1.ReviewKycRequest{Id: id, Decision: "reject"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("second review: want FailedPrecondition, got %v", err)
	}
	st, err := h.GetVerificationStatus(buyer("alice"), &verificationv1.GetVerificationStatusRequest{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("first decision overwritten: %v", st.GetStatus())
	}
}
