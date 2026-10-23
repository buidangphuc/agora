package verification_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
	"github.com/buidangphuc/team-verification/internal/verification"
)

func newSvc() *verification.VerificationService {
	return verification.NewVerificationService(storetest.NewInMemoryKycRepo())
}

// Submitting a KYC document creates a PENDING record with no badge yet.
func TestSubmitCreatesPending(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	sub, err := svc.Submit(ctx, "user_a", "national_id", "s3://mock/ref-123")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if sub.Status != store.StatusPending {
		t.Fatalf("expected PENDING, got %s", sub.Status)
	}
	if sub.ID == "" {
		t.Fatalf("expected a generated id")
	}

	st, badge, err := svc.GetStatus(ctx, "user_a")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if st != store.StatusPending || badge {
		t.Fatalf("expected PENDING/no-badge, got %s badge=%v", st, badge)
	}
}

// Approving a submission moves it to VERIFIED and earns the badge.
func TestReviewApproveSetsBadge(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	sub, err := svc.Submit(ctx, "user_a", "passport", "ref-abc")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	st, err := svc.Review(ctx, sub.ID, "approve")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if st != store.StatusVerified {
		t.Fatalf("expected VERIFIED, got %s", st)
	}

	got, badge, err := svc.GetStatus(ctx, "user_a")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got != store.StatusVerified || !badge {
		t.Fatalf("expected VERIFIED/badge=true, got %s badge=%v", got, badge)
	}
}

// Rejecting a submission moves it to REJECTED and grants no badge.
func TestReviewRejectPath(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	sub, err := svc.Submit(ctx, "user_b", "business_license", "ref-xyz")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	st, err := svc.Review(ctx, sub.ID, "reject")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if st != store.StatusRejected {
		t.Fatalf("expected REJECTED, got %s", st)
	}

	got, badge, err := svc.GetStatus(ctx, "user_b")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got != store.StatusRejected || badge {
		t.Fatalf("expected REJECTED/no-badge, got %s badge=%v", got, badge)
	}
}

// A user who never submitted is reported PENDING with no badge, not an error.
func TestGetStatusUnknownUserReturnsPending(t *testing.T) {
	svc := newSvc()
	st, badge, err := svc.GetStatus(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != store.StatusPending || badge {
		t.Fatalf("expected PENDING/no-badge for unknown user, got %s badge=%v", st, badge)
	}
}

// Input validation: empty user, empty fields, bad decision, missing id.
func TestValidation(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	if _, err := svc.Submit(ctx, "", "national_id", "ref"); err != verification.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser, got %v", err)
	}
	if _, err := svc.Submit(ctx, "u", "", "ref"); err != verification.ErrEmptyDocType {
		t.Fatalf("expected ErrEmptyDocType, got %v", err)
	}
	if _, err := svc.Submit(ctx, "u", "national_id", "  "); err != verification.ErrEmptyDocRef {
		t.Fatalf("expected ErrEmptyDocRef, got %v", err)
	}
	if _, _, err := svc.GetStatus(ctx, ""); err != verification.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser on GetStatus, got %v", err)
	}
	if _, err := svc.Review(ctx, "", "approve"); err != verification.ErrEmptyID {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}
	if _, err := svc.Review(ctx, "kyc_x", "maybe"); err != verification.ErrInvalidReview {
		t.Fatalf("expected ErrInvalidReview, got %v", err)
	}
	if _, err := svc.Review(ctx, "kyc_missing", "approve"); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound reviewing missing id, got %v", err)
	}
}
