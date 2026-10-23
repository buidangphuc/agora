package rpc_test

import (
	"context"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/rpc"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
	"github.com/buidangphuc/team-verification/internal/verification"
)

func buildHandler(t testing.TB, repo store.KycRepository, w auditx.Writer, logger *slog.Logger) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(verification.NewVerificationService(repo), w, logger)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func newHandler() *rpc.Handler {
	return buildHandler(nopT{}, storetest.NewInMemoryKycRepo(), &fakeAudit{}, quiet())
}

type nopT struct{ testing.TB }

func (nopT) Helper()                   {}
func (nopT) Fatalf(_ string, _ ...any) { panic("NewHandler failed") }

// ctxAs resolves a principal exactly as the gRPC server does: forwarded
// x-principal-* metadata run through the SDK unary interceptor.
func ctxAs(id, ptype, scopes string) context.Context {
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", ptype, "x-principal-scopes", scopes)
	in := metadata.NewIncomingContext(context.Background(), md)
	var out context.Context
	_, _ = auth.UnaryServerInterceptor()(in, nil, &grpc.UnaryServerInfo{},
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

func wantMessage(t *testing.T, err error, want string) {
	t.Helper()
	if got := status.Convert(err).Message(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func submit(ctx context.Context, t *testing.T, h *rpc.Handler) string {
	t.Helper()
	sub, err := h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "mock-ref"})
	if err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
	return sub.GetId()
}

// Submit -> PENDING, an admin approves -> VERIFIED + badge for the owner.
// Scenario: Submit then review through the handler.
func TestHandlerSubmitReviewFlow(t *testing.T) {
	h := newHandler()
	id := submit(buyer("seller_1"), t, h)

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

// Scenario: Two users do not see each other's status.
func TestTwoUsersDoNotSeeEachOthersStatus(t *testing.T) {
	h := newHandler()
	id := submit(buyer("alice"), t, h)
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

// Scenario: A service principal cannot submit.
func TestServicePrincipalCannotSubmit(t *testing.T) {
	_, err := newHandler().SubmitKyc(service1(), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
	wantCode(t, err, codes.PermissionDenied)
}

func TestBuyerCannotReview(t *testing.T) {
	h := newHandler()
	id := submit(buyer("alice"), t, h)
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

// Scenario: An admin can review someone else.
func TestAdminCanReviewSomeoneElse(t *testing.T) {
	h := newHandler()
	id := submit(buyer("alice"), t, h)
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
	id := submit(admin("admin_1"), t, h)
	_, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	wantCode(t, err, codes.PermissionDenied)
	st, _ := h.GetVerificationStatus(admin("admin_1"), &verificationv1.GetVerificationStatusRequest{})
	if st.GetBadge() {
		t.Fatalf("self-review must not verify the submitter")
	}
}

func TestStatusForAnotherUser(t *testing.T) {
	h := newHandler()
	id := submit(buyer("alice"), t, h)
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
	id := submit(buyer("u"), t, h)
	_, err := h.ReviewKyc(admin("admin_1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "meh"})
	wantCode(t, err, codes.InvalidArgument)
}

// A decision is final: a second review (or a concurrent one that loses) gets
// FailedPrecondition and leaves the first decision in place.
// Scenario: A review decision is final through the handler.
func TestReviewIsFinal(t *testing.T) {
	h := newHandler()
	id := submit(buyer("alice"), t, h)
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
