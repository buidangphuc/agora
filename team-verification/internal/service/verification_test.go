package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
)

func newSvc() *service.VerificationService {
	return service.NewVerificationService(repository.NewInMemoryKycRepo())
}

// Submitting a KYC document creates a PENDING record with no badge yet.
func TestSubmitCreatesPending(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	sub, err := svc.Submit(ctx, "user_a", "national_id", "s3://mock/ref-123")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if sub.Status != repository.StatusPending {
		t.Fatalf("expected PENDING, got %s", sub.Status)
	}
	if sub.ID == "" {
		t.Fatalf("expected a generated id")
	}

	st, badge, err := svc.GetStatus(ctx, "user_a")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if st != repository.StatusPending || badge {
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
	if st != repository.StatusVerified {
		t.Fatalf("expected VERIFIED, got %s", st)
	}

	got, badge, err := svc.GetStatus(ctx, "user_a")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got != repository.StatusVerified || !badge {
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
	if st != repository.StatusRejected {
		t.Fatalf("expected REJECTED, got %s", st)
	}

	got, badge, err := svc.GetStatus(ctx, "user_b")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if got != repository.StatusRejected || badge {
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
	if st != repository.StatusPending || badge {
		t.Fatalf("expected PENDING/no-badge for unknown user, got %s badge=%v", st, badge)
	}
}

// Input validation: empty user, empty fields, bad decision, missing id.
func TestValidation(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	if _, err := svc.Submit(ctx, "", "national_id", "ref"); err != service.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser, got %v", err)
	}
	if _, err := svc.Submit(ctx, "u", "", "ref"); err != service.ErrEmptyDocType {
		t.Fatalf("expected ErrEmptyDocType, got %v", err)
	}
	if _, err := svc.Submit(ctx, "u", "national_id", "  "); err != service.ErrEmptyDocRef {
		t.Fatalf("expected ErrEmptyDocRef, got %v", err)
	}
	if _, _, err := svc.GetStatus(ctx, ""); err != service.ErrEmptyUser {
		t.Fatalf("expected ErrEmptyUser on GetStatus, got %v", err)
	}
	if _, err := svc.Review(ctx, "", "approve"); err != service.ErrEmptyID {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}
	if _, err := svc.Review(ctx, "kyc_x", "maybe"); err != service.ErrInvalidReview {
		t.Fatalf("expected ErrInvalidReview, got %v", err)
	}
	if _, err := svc.Review(ctx, "kyc_missing", "approve"); err != repository.ErrNotFound {
		t.Fatalf("expected ErrNotFound reviewing missing id, got %v", err)
	}
}
