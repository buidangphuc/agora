package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

// mockOrderVerifier is a stand-in for internal/upstream.OrderClient so review
// enrichment can be tested without team-order.
type mockOrderVerifier struct {
	verified bool
	sellerID string
	err      error
	calls    int
}

func (m *mockOrderVerifier) VerifyPurchase(_ context.Context, _, _, _ string) (bool, string, error) {
	m.calls++
	return m.verified, m.sellerID, m.err
}

func TestCreateReviewPersistsMediaAndVerifiedPurchase(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	verifier := &mockOrderVerifier{verified: true, sellerID: "seller-1"}
	svc := service.NewReviewService(repo, verifier, nil)

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "order-1", 5, "Tuyệt vời",
		[]string{"https://cdn/x.jpg", "https://cdn/y.jpg"})
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if len(rev.MediaURLs) != 2 {
		t.Fatalf("expected 2 media urls, got %v", rev.MediaURLs)
	}
	if !rev.VerifiedPurchase {
		t.Fatal("expected verified_purchase true when order confirms")
	}
	if rev.SellerID != "seller-1" {
		t.Fatalf("expected seller-1, got %q", rev.SellerID)
	}
	if verifier.calls != 1 {
		t.Fatalf("expected verifier called once, got %d", verifier.calls)
	}
}

func TestCreateReviewUnverifiedWhenOrderDenies(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	verifier := &mockOrderVerifier{verified: false, sellerID: ""}
	svc := service.NewReviewService(repo, verifier, nil)

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "order-1", 4, "Ổn", nil)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if rev.VerifiedPurchase {
		t.Fatal("expected verified_purchase false when order does not confirm")
	}
}

func TestCreateReviewDegradesWhenVerifierErrors(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	verifier := &mockOrderVerifier{err: errors.New("order unavailable")}
	svc := service.NewReviewService(repo, verifier, nil)

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "order-1", 5, "", nil)
	if err != nil {
		t.Fatalf("CreateReview should degrade, not fail: %v", err)
	}
	if rev.VerifiedPurchase {
		t.Fatal("expected verified_purchase false on verifier error")
	}
}

func TestCreateReviewNoVerifierNoCall(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	svc := service.NewReviewService(repo, nil, nil) // verifier disabled

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "order-1", 5, "", nil)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if rev.VerifiedPurchase {
		t.Fatal("expected verified_purchase false when no verifier configured")
	}
}

func TestMarkHelpfulIdempotentPerUser(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	svc := service.NewReviewService(repo, nil, nil)

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "", 5, "", nil)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	// Same user voting twice counts once.
	c1, err := svc.MarkHelpful(ctx, rev.ID, "user-a")
	if err != nil {
		t.Fatalf("MarkHelpful a1: %v", err)
	}
	if c1 != 1 {
		t.Fatalf("expected count 1, got %d", c1)
	}
	c2, err := svc.MarkHelpful(ctx, rev.ID, "user-a")
	if err != nil {
		t.Fatalf("MarkHelpful a2: %v", err)
	}
	if c2 != 1 {
		t.Fatalf("expected idempotent count 1, got %d", c2)
	}

	// A different user increments.
	c3, err := svc.MarkHelpful(ctx, rev.ID, "user-b")
	if err != nil {
		t.Fatalf("MarkHelpful b: %v", err)
	}
	if c3 != 2 {
		t.Fatalf("expected count 2, got %d", c3)
	}

	// Unknown review → not found.
	if _, err := svc.MarkHelpful(ctx, "no-such-review", "user-a"); !errors.Is(err, repository.ErrReviewNotFound) {
		t.Fatalf("expected ErrReviewNotFound, got %v", err)
	}
}

func TestGetShopRatingSummaryAcrossListings(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryReviewRepository()
	verifier := &mockOrderVerifier{verified: true, sellerID: "seller-1"}
	svc := service.NewReviewService(repo, verifier, nil)

	// Two listings, same seller, ratings 5,3 (listing-1) and 4 (listing-2).
	if _, err := svc.CreateReview(ctx, "listing-1", "b1", "B1", "o1", 5, "", nil); err != nil {
		t.Fatalf("review1: %v", err)
	}
	if _, err := svc.CreateReview(ctx, "listing-1", "b2", "B2", "o2", 3, "", nil); err != nil {
		t.Fatalf("review2: %v", err)
	}
	if _, err := svc.CreateReview(ctx, "listing-2", "b3", "B3", "o3", 4, "", nil); err != nil {
		t.Fatalf("review3: %v", err)
	}
	// A review for a different seller must not leak in.
	other := &mockOrderVerifier{verified: true, sellerID: "seller-2"}
	otherSvc := service.NewReviewService(repo, other, nil)
	if _, err := otherSvc.CreateReview(ctx, "listing-9", "b9", "B9", "o9", 1, "", nil); err != nil {
		t.Fatalf("review-other: %v", err)
	}

	sum, err := svc.GetShopRatingSummary(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetShopRatingSummary: %v", err)
	}
	if sum.ReviewCount != 3 {
		t.Fatalf("expected 3 reviews for seller-1, got %d", sum.ReviewCount)
	}
	if sum.AverageRating != 4.0 { // (5+3+4)/3
		t.Fatalf("expected avg 4.0, got %v", sum.AverageRating)
	}
	if sum.Breakdown.Star5 != 1 || sum.Breakdown.Star4 != 1 || sum.Breakdown.Star3 != 1 {
		t.Fatalf("unexpected breakdown: %+v", sum.Breakdown)
	}

	// Validation
	if _, err := svc.GetShopRatingSummary(ctx, ""); !errors.Is(err, service.ErrEmptySeller) {
		t.Fatalf("expected ErrEmptySeller, got %v", err)
	}
}
