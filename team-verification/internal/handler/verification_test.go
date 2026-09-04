package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/handler"
	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
)

func newHandler() *handler.VerificationHandler {
	svc := service.NewVerificationService(repository.NewInMemoryKycRepo())
	return handler.NewVerificationHandler(svc)
}

// ctxAs forwards a principal id the way the gateway would, so the handler
// resolves an auth-scoped user rather than the demo fallback.
func ctxAs(userID string) context.Context {
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("x-principal-id", userID),
	)
}

// Full gRPC path: submit -> PENDING, approve -> VERIFIED + badge, scoped to the
// forwarded principal.
func TestHandlerSubmitReviewFlow(t *testing.T) {
	h := newHandler()
	ctx := ctxAs("seller_1")

	sub, err := h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{
		DocType: "national_id",
		DocRef:  "mock-ref-1",
	})
	if err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
	if sub.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("expected PENDING, got %v", sub.GetStatus())
	}

	// Before review: status PENDING, no badge.
	st, err := h.GetVerificationStatus(ctx, &verificationv1.GetVerificationStatusRequest{})
	if err != nil {
		t.Fatalf("GetVerificationStatus: %v", err)
	}
	if st.GetBadge() {
		t.Fatalf("expected no badge before review")
	}

	// Approve.
	rev, err := h.ReviewKyc(ctx, &verificationv1.ReviewKycRequest{Id: sub.GetId(), Decision: "approve"})
	if err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("expected VERIFIED, got %v", rev.GetStatus())
	}

	// After review: VERIFIED + badge, looked up by explicit user_id.
	st2, err := h.GetVerificationStatus(ctx, &verificationv1.GetVerificationStatusRequest{UserId: "seller_1"})
	if err != nil {
		t.Fatalf("GetVerificationStatus: %v", err)
	}
	if st2.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED || !st2.GetBadge() {
		t.Fatalf("expected VERIFIED/badge, got %v badge=%v", st2.GetStatus(), st2.GetBadge())
	}
}

// Reviewing a non-existent submission surfaces gRPC NotFound.
func TestHandlerReviewMissingIsNotFound(t *testing.T) {
	h := newHandler()
	_, err := h.ReviewKyc(context.Background(), &verificationv1.ReviewKycRequest{Id: "kyc_missing", Decision: "approve"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

// An invalid review decision surfaces gRPC InvalidArgument.
func TestHandlerBadDecisionIsInvalidArgument(t *testing.T) {
	h := newHandler()
	sub, err := h.SubmitKyc(ctxAs("u"), &verificationv1.SubmitKycRequest{DocType: "passport", DocRef: "r"})
	if err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
	_, err = h.ReviewKyc(context.Background(), &verificationv1.ReviewKycRequest{Id: sub.GetId(), Decision: "meh"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}
